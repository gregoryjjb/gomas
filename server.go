package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/rs/zerolog/log"
)

/////////////////////
// Response helpers

func RespondInternalServiceError(w http.ResponseWriter, err error) {
	w.WriteHeader(http.StatusInternalServerError)
	w.Write([]byte(err.Error()))
}

func RespondNotFoundError(w http.ResponseWriter, body string) {
	w.WriteHeader(http.StatusNotFound)
	if body == "" {
		body = "Not found"
	}
	RespondText(w, body)
}

func RespondBadRequest(w http.ResponseWriter, message string) {
	w.WriteHeader(http.StatusBadRequest)
	RespondText(w, message)
}

func RespondText(w http.ResponseWriter, body string) {
	w.Write([]byte(body))
}

func RespondJSON(w http.ResponseWriter, body any) {
	w.Header().Add("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(body); err != nil {
		RespondInternalServiceError(w, err)
	}
}

// FileServer conveniently sets up a http.FileServer handler to serve
// static files from a http.FileSystem.
func FileServer(r chi.Router, path string, root http.FileSystem) {
	if strings.ContainsAny(path, "{}*") {
		panic("FileServer does not permit any URL parameters.")
	}

	if path != "/" && path[len(path)-1] != '/' {
		r.Get(path, http.RedirectHandler(path+"/", 301).ServeHTTP)
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

func StartServer(player *Player) error {
	// Ensure templates parse correctly
	_, err := GetTemplates()
	if err != nil {
		return err
	}

	r := chi.NewRouter()

	r.Use(middleware.Recoverer)
	r.Use(middleware.NoCache)
	r.Use(LoggerMiddleware(&log.Logger))

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		shows, err := ListShows()
		if err != nil {
			RespondInternalServiceError(w, err)
			return
		}

		playlists, err := ListPlaylists()
		if err != nil {
			RespondInternalServiceError(w, err)
			return
		}

		pagedata := struct {
			Shows     []*ShowInfo
			Playlists []Playlist
			Exist     map[string]bool
		}{shows, playlists, make(map[string]bool)}

		for _, show := range shows {
			pagedata.Exist[show.ID] = true
		}

		tmpl, err := GetTemplates()
		if err != nil {
			RespondInternalServiceError(w, err)
			return
		}

		tmpl.ExecuteTemplate(w, "index.html", pagedata)
	})

	r.Get("/config", func(w http.ResponseWriter, r *http.Request) {
		t, err := GetTemplates()
		if err != nil {
			RespondInternalServiceError(w, err)
			return
		}

		t.ExecuteTemplate(w, "config.html", struct{ Version string }{version})
	})

	// Cram the legacy show editor in
	FileServer(r, "/editor", GetEditorFS())

	r.Get("/logs", func(w http.ResponseWriter, r *http.Request) {
		t, err := GetTemplates()
		if err != nil {
			RespondInternalServiceError(w, err)
			return
		}

		t.ExecuteTemplate(w, "logs.html", BufferedLogsArray())
	})

	// For testing panics
	r.Get("/panic", func(w http.ResponseWriter, r *http.Request) {
		panic("oh no what happened!")
	})

	api := chi.NewRouter()

	// GET shows
	api.Get("/shows", func(w http.ResponseWriter, r *http.Request) {
		shows, err := ListShows()
		if err != nil {
			RespondInternalServiceError(w, err)
			return
		}

		RespondJSON(w, shows)
	})

	// GET single show
	api.Get("/shows/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		show, err := LoadShow(id)
		if err != nil {
			RespondInternalServiceError(w, err)
			return
		}

		RespondJSON(w, show)
	})

	// POST new shot
	api.Post("/shows", func(w http.ResponseWriter, r *http.Request) {
		err := r.ParseMultipartForm(32 << 20) // 32MB
		if err != nil {
			RespondBadRequest(w, err.Error())
			return
		}

		id := r.PostFormValue("id")
		if id == "" {
			RespondBadRequest(w, "id is required")
			return
		}
		if err := ValidateShowID(id); err != nil {
			RespondBadRequest(w, err.Error())
			return
		}
		if ShowExists(id) {
			RespondBadRequest(w, fmt.Sprintf("show already exists: %s", id))
			return
		}

		audioFile, h, err := r.FormFile("audio_file")
		if err != nil {
			RespondBadRequest(w, "audio_file is required")
			return
		}
		if filepath.Ext(h.Filename) != ".mp3" {
			RespondBadRequest(w, "audio_file must be an .mp3")
			return
		}

		show := NewShow(id)
		if err := SaveShow(id, show); err != nil {
			RespondInternalServiceError(w, err)
			return
		}
		if err := SaveShowAudio(id, audioFile); err != nil {
			RespondInternalServiceError(w, err)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	})

	// PUT single show
	api.Put("/shows/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if id == "" {
			RespondNotFoundError(w, "show id required")
			return
		}

		var show LegacyShow
		if err := json.NewDecoder(r.Body).Decode(&show); err != nil {
			RespondInternalServiceError(w, err)
			return
		}

		if !ShowExists(id) {
			RespondNotFoundError(w, fmt.Sprintf("show '%s' not found", id))
			return
		}

		if len(show.Tracks) == 0 {
			RespondBadRequest(w, "show had zero tracks, refusing to save!")
			return
		}

		if err := SaveShow(id, &show); err != nil {
			RespondInternalServiceError(w, err)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	})

	// GET audio file for single show
	api.Get("/audio/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		audioPath, err := ShowAudioPath(id)
		if err != nil {
			RespondNotFoundError(w, "audio not found")
			return
		}
		http.ServeFile(w, r, audioPath)
	})

	// GET Playlists
	api.Get("/playlists", func(w http.ResponseWriter, r *http.Request) {
		playlists, err := ListPlaylists()
		if err != nil {
			RespondInternalServiceError(w, err)
		}

		RespondJSON(w, playlists)
	})

	// Play single show
	api.Get("/play/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		exists, err := ShowIsPlayable(id)
		if err != nil {
			RespondInternalServiceError(w, err)
			return
		}
		if !exists {
			RespondNotFoundError(w, fmt.Sprintf("show '%s' does not exist", id))
			return
		}

		player.Play(id)

		w.WriteHeader(http.StatusNoContent)
	})

	// Play all shows
	api.Get("/playall", func(w http.ResponseWriter, r *http.Request) {
		player.PlayAll()
		w.WriteHeader(http.StatusNoContent)
	})

	// Set static state
	api.Get("/static", func(w http.ResponseWriter, r *http.Request) {
		player.Stop()
		w.WriteHeader(http.StatusNoContent)
	})

	// Play next show in the queue
	api.Get("/next", func(w http.ResponseWriter, r *http.Request) {
		player.Next()
		w.WriteHeader(http.StatusNoContent)
	})

	api.Get("/logs", func(w http.ResponseWriter, r *http.Request) {
		BufferedLogs(w)
	})

	r.Mount("/api", api)

	r.Get("/ws", createWebsocketHandler(player))

	staticFS, err := GetStatic()
	if err != nil {
		return err
	}
	// r.Method("GET", "/static", http.StripPrefix("/static/", ))
	r.Handle("/*", http.FileServer(http.FS(staticFS)))

	address := fmt.Sprintf("%s:%s", Host, Port)
	log.Info().Str("listen", address).Msg("launching server")
	return http.ListenAndServe(address, r)
}
