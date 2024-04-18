package main

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"gregoryjjb/gomas/gpio"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/rs/zerolog/log"
	"github.com/spf13/afero"
)

////////////////
// API helpers

func NameParam(r *http.Request) (string, error) {
	v := chi.URLParam(r, "name")
	if v == "" {
		return "", fmt.Errorf("name is required")
	}
	if err := ValidateShowName(v); err != nil {
		return "", err
	}
	return v, nil
}

var ErrValidation = errors.New("validation failed")

func validationErr(err error) error {
	return fmt.Errorf("%w: %w", ErrValidation, err)
}

type HandlerWithError func(http.ResponseWriter, *http.Request) error

func (h HandlerWithError) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if err := h(w, r); err != nil {
		var status int

		switch {
		case
			errors.Is(err, os.ErrNotExist),
			errors.Is(err, afero.ErrFileNotFound),
			errors.Is(err, ErrNotExist):
			status = http.StatusNotFound
		case
			errors.Is(err, ErrValidation),
			errors.Is(err, ErrExists):
			status = http.StatusBadRequest
		default:
			status = http.StatusInternalServerError
		}

		w.WriteHeader(status)
		w.Write([]byte(err.Error()))
	}
}

func RespondJSON(w http.ResponseWriter, body any) error {
	w.Header().Add("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(body)
}

// FileServer conveniently sets up a http.FileServer handler to serve
// static files from a http.FileSystem.
func FileServer(r chi.Router, path string, root http.FileSystem) {
	if strings.ContainsAny(path, "{}*") {
		panic("FileServer does not permit any URL parameters.")
	}

	if path != "/" && path[len(path)-1] != '/' {
		r.Get(path, http.RedirectHandler(path+"/", http.StatusMovedPermanently).ServeHTTP)
		path += "/"
	}
	path += "*"

	r.Get(path, func(w http.ResponseWriter, r *http.Request) {
		rctx := chi.RouteContext(r.Context())
		pathPrefix := strings.TrimSuffix(rctx.RoutePattern(), "/*")
		fs := http.StripPrefix(pathPrefix, http.FileServer(root))
		fs.ServeHTTP(w, r)
	})
}

func StartServer(config *Config, buildInfo BuildInfo, player *Player, storage *Storage) error {
	// Ensure templates parse correctly
	_, err := GetTemplates(config.DisableEmbed)
	if err != nil {
		return err
	}

	r := chi.NewRouter()
	get := func(pattern string, handler HandlerWithError) {
		r.Method(http.MethodGet, pattern, handler)
	}
	post := func(pattern string, handler HandlerWithError) {
		r.Method(http.MethodPost, pattern, handler)
	}
	put := func(pattern string, handler HandlerWithError) {
		r.Method(http.MethodPut, pattern, handler)
	}

	r.Use(middleware.Recoverer)
	r.Use(middleware.NoCache)
	r.Use(LoggerMiddleware(&log.Logger))

	get("/", func(w http.ResponseWriter, r *http.Request) error {
		shows, err := storage.ListShows()
		if err != nil {
			return err
		}

		playlists, err := storage.ListPlaylists()
		if err != nil {
			return err
		}

		pagedata := struct {
			Shows     []string
			Playlists []Playlist
			Exist     map[string]bool
		}{shows, playlists, make(map[string]bool)}

		for _, show := range shows {
			pagedata.Exist[show] = true
		}

		tmpl, err := GetTemplates(config.DisableEmbed)
		if err != nil {
			return err
		}

		return tmpl.ExecuteTemplate(w, "index.html", pagedata)
	})

	get("/config", func(w http.ResponseWriter, r *http.Request) error {
		t, err := GetTemplates(config.DisableEmbed)
		if err != nil {
			return err
		}

		return t.ExecuteTemplate(w, "config.html", struct{ Version string }{version})
	})

	// Cram the legacy show editor in
	FileServer(r, "/editor", GetEditorFS(config.DisableEmbed))

	get("/logs", func(w http.ResponseWriter, r *http.Request) error {
		t, err := GetTemplates(config.DisableEmbed)
		if err != nil {
			return err
		}

		return t.ExecuteTemplate(w, "logs.html", BufferedLogsArray())
	})

	// For testing panics
	get("/panic", func(w http.ResponseWriter, r *http.Request) error {
		panic("oh no what happened!")
	})

	get("/api", func(w http.ResponseWriter, r *http.Request) error {
		return RespondJSON(w, map[string]any{
			"version":     buildInfo.Version,
			"built_at":    buildInfo.Time,
			"commit_hash": buildInfo.CommitHash,
		})
	})

	// GET shows
	get("/api/shows", func(w http.ResponseWriter, r *http.Request) error {
		shows, err := storage.ListShows()
		if err != nil {
			return err
		}

		return RespondJSON(w, shows)
	})

	// GET single show
	get("/api/shows/{name}", func(w http.ResponseWriter, r *http.Request) error {
		name := chi.URLParam(r, "name")
		show, err := storage.ReadShowData(name)
		if err != nil {
			return err
		}

		return RespondJSON(w, show)
	})

	// POST new show
	post("/api/shows", func(w http.ResponseWriter, r *http.Request) error {
		err := r.ParseMultipartForm(32 << 20) // 32MB
		if err != nil {
			return validationErr(err)
		}

		name := r.PostFormValue("name")
		if err := storage.ShowNotExists(name); err != nil {
			return err
		}

		audioFile, h, err := r.FormFile("audio_file")
		if err != nil {
			return validationErr(err)
		}
		if filepath.Ext(h.Filename) != ".mp3" {
			return validationErr(errors.New("audio_file must be an .mp3"))
		}

		data := NewProjectData(8)

		if err := storage.CreateShow(name, data, audioFile); err != nil {
			return err
		}

		return nil
	})

	// POST a shmr file, creating a new show
	post("/api/upload", func(w http.ResponseWriter, r *http.Request) error {
		r.ParseMultipartForm(50 << 20) // 50 MiB max

		file, header, err := r.FormFile("file")
		if err != nil {
			return validationErr(err)
		}
		defer file.Close()

		return storage.ImportShmr(file, header)
	})

	// GET a shmr file from existing show
	get("/api/export/{name}", func(w http.ResponseWriter, r *http.Request) error {
		name := chi.URLParam(r, "name")
		if err := storage.ShowExists(name); err != nil {
			return err
		}

		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s.shmr\"", name))
		return storage.ExportShmr(name, w)
	})

	// PUT single show
	put("/api/shows/{name}", func(w http.ResponseWriter, r *http.Request) error {
		name := chi.URLParam(r, "name")
		if err := storage.ShowExists(name); err != nil {
			return err
		}

		var show ProjectData
		if err := json.NewDecoder(r.Body).Decode(&show); err != nil {
			return err
		}

		if len(show.Tracks) == 0 {
			return fmt.Errorf("%w: show had zero tracks, refusing to save", ErrValidation)
		}

		return storage.WriteShowData(name, show)
	})

	// PUT rename a show
	put("/api/rename", func(w http.ResponseWriter, r *http.Request) error {
		from := r.URL.Query().Get("from")
		to := r.URL.Query().Get("to")

		return storage.RenameShow(from, to)
	})

	// GET audio file for single show
	get("/api/audio/{name}", func(w http.ResponseWriter, r *http.Request) error {
		name := chi.URLParam(r, "name")
		audio, err := storage.ReadAudio(name)
		if err != nil {
			return err
		}

		info, err := audio.Stat()
		if err != nil {
			return err
		}

		http.ServeContent(w, r, audio.Name(), info.ModTime(), audio)
		return nil
	})

	// GET Playlists
	get("/api/playlists", func(w http.ResponseWriter, r *http.Request) error {
		playlists, err := storage.ListPlaylists()
		if err != nil {
			return err
		}

		return RespondJSON(w, playlists)
	})

	// Play single show
	get("/api/play/{name}", func(w http.ResponseWriter, r *http.Request) error {
		name := chi.URLParam(r, "name")
		if err := storage.ShowExists(name); err != nil {
			return err
		}

		player.Play(name)

		w.WriteHeader(http.StatusNoContent)
		return nil
	})

	// Play all shows
	get("/api/playall", func(w http.ResponseWriter, r *http.Request) error {
		player.PlayAll()
		w.WriteHeader(http.StatusNoContent)
		return nil
	})

	// Set static state
	get("/api/static", func(w http.ResponseWriter, r *http.Request) error {
		state := r.URL.Query().Get("value") == "1"

		player.Stop()

		// This is hacky but I want to be sure the player is not executing a frame
		time.Sleep(time.Millisecond * 10)
		gpio.SetAll(state)

		w.WriteHeader(http.StatusNoContent)
		return nil
	})

	// Play next show in the queue
	get("/api/next", func(w http.ResponseWriter, r *http.Request) error {
		player.Next()
		w.WriteHeader(http.StatusNoContent)
		return nil
	})

	get("/api/logs", func(w http.ResponseWriter, r *http.Request) error {
		BufferedLogs(w)
		return nil
	})

	r.Get("/ws", createWebsocketHandler(player))

	staticFS, err := GetStaticFS(config.DisableEmbed)
	if err != nil {
		return err
	}
	FileServer(r, "/", staticFS)

	chi.Walk(r, func(method string, route string, handler http.Handler, middlewares ...func(http.Handler) http.Handler) error {
		log.Debug().Str("method", method).Str("route", route).Int("middleware_count", len(middlewares)).Msg("Registered route")
		return nil
	})

	address := fmt.Sprintf("%s:%s", config.Host, config.Port)
	log.Info().Str("listen", address).Msg("launching server")
	return http.ListenAndServe(address, r)
}
