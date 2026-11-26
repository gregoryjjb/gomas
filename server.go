package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json/v2"
	"errors"
	"fmt"
	"gregoryjjb/gomas/gpio"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
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
	return json.MarshalWrite(w, body)
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

func StartServer(ctx context.Context, config *Config, buildInfo BuildInfo, player *Player, storage *Storage, registry *SlaveRegistry) error {
	handler, err := NewServerHandler(config, buildInfo, player, storage, registry)
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp", net.JoinHostPort(config.Host(), config.Port()))
	if err != nil {
		return err
	}
	defer listener.Close()

	stopDiscovery, err := registry.Start(ctx, config.Role(), listener.Addr().(*net.TCPAddr))
	if err != nil {
		return fmt.Errorf("start slave discovery: %w", err)
	}
	defer stopDiscovery()

	server := &http.Server{Handler: handler}
	stopClosing := context.AfterFunc(ctx, func() { server.Close() })
	defer stopClosing()
	log.Info().Str("listen", listener.Addr().String()).Msg("launching server")
	err = server.Serve(listener)
	if ctx.Err() != nil && errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

// NewServerHandler constructs the routes shared by production and test servers.
func NewServerHandler(config *Config, buildInfo BuildInfo, player *Player, storage *Storage, registry *SlaveRegistry) (http.Handler, error) {
	// Ensure templates parse correctly
	_, err := GetTemplates()
	if err != nil {
		return nil, err
	}
	hostname, err := os.Hostname()
	if err != nil {
		return nil, fmt.Errorf("get hostname: %w", err)
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
	r.Use(middleware.Compress(5))
	r.Use(LoggerMiddleware(&log.Logger))
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins: []string{"http://localhost:5173"},
		AllowedMethods: []string{"GET", "POST", "PUT"},
	}))

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
			Hostname  string
			Role      Role
			Slaves    map[string]Slave
			Shows     []string
			Playlists []Playlist
			Exist     map[string]bool
		}{Hostname: hostname, Role: config.Role(), Shows: shows, Playlists: playlists, Exist: make(map[string]bool)}
		if pagedata.Role == RoleMaster {
			pagedata.Slaves = registry.Snapshot()
		}

		for _, show := range shows {
			pagedata.Exist[show] = true
		}

		tmpl, err := GetTemplates()
		if err != nil {
			return err
		}

		return tmpl.ExecuteTemplate(w, "index.html", pagedata)
	})

	get("/config", func(w http.ResponseWriter, r *http.Request) error {
		t, err := GetTemplates()
		if err != nil {
			return err
		}

		return t.ExecuteTemplate(w, "config.html", struct {
			Version    string
			ConfigToml string
			ConfigPath string
		}{buildInfo.Version, config.RawToml(), config.tomlPath})
	})

	// Cram the legacy show editor in
	FileServer(r, "/editor", GetEditorFS())

	get("/logs", func(w http.ResponseWriter, r *http.Request) error {
		t, err := GetTemplates()
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

		data := NewShowData(config.Tracks())

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

	// GET show data in JSON format
	get("/api/shows/{name}/data", func(w http.ResponseWriter, r *http.Request) error {
		name := chi.URLParam(r, "name")

		if r.URL.Query().Get("raw") == "true" {
			file, err := storage.ReadRawShowData(name)
			if err != nil {
				return fmt.Errorf("read raw show data: %w", err)
			}
			defer file.Close()
			w.Header().Set("Content-Type", "application/json")
			_, err = io.Copy(w, file)
			return err
		}

		data, err := storage.ReadShowData(name)
		if err != nil {
			return fmt.Errorf("read show data: %w", err)
		}

		return RespondJSON(w, data)
	})

	// PUT single show
	put("/api/shows/{name}/data", func(w http.ResponseWriter, r *http.Request) error {
		name := chi.URLParam(r, "name")
		if err := storage.ShowExists(name); err != nil {
			return err
		}

		show, err := DecodeShow(r.Body)
		if err != nil {
			return err
		}

		if len(show.Tracks) == 0 {
			return fmt.Errorf("%w: show had zero tracks, refusing to save", ErrValidation)
		}

		return storage.WriteShowData(name, show)
	})

	// GET audio file for single show
	get("/api/shows/{name}/audio", func(w http.ResponseWriter, r *http.Request) error {
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

	// PUT rename a show
	put("/api/rename", func(w http.ResponseWriter, r *http.Request) error {
		from := r.URL.Query().Get("from")
		to := r.URL.Query().Get("to")

		if from == "" || to == "" {
			return fmt.Errorf("query params 'from' and 'to' required")
		}

		return storage.RenameShow(from, to)
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

	get("/api/playSlave", func(w http.ResponseWriter, r *http.Request) error {
		if config.Role() != RoleSlave {
			return fmt.Errorf("%w: slave role required, found: %q", ErrValidation, config.Role())
		}

		query := r.URL.Query()
		name := query.Get("name")
		if err := ValidateShowName(name); err != nil {
			return err
		}
		hash := query.Get("sha256")
		digest, err := hex.DecodeString(hash)
		if err != nil || len(digest) != 32 {
			return fmt.Errorf("%w: sha256 must be a 64-character hex digest", ErrValidation)
		}
		hash = strings.ToLower(hash)

		startTimeStr := query.Get("slaveStartTimeMicro")
		if startTimeStr == "" {
			return fmt.Errorf("%w: slaveStartTimeMicro is required", ErrValidation)
		}
		startTime, err := UnmarshalStartTime(startTimeStr)
		if err != nil || startTime.IsZero() {
			return fmt.Errorf("%w: slaveStartTimeMicro must be a valid Unix microsecond timestamp", ErrValidation)
		}

		masterPort, err := strconv.Atoi(query.Get("masterPort"))
		if err != nil || masterPort < 1 || masterPort > 65535 {
			return fmt.Errorf("%w: masterPort must be between 1 and 65535", ErrValidation)
		}
		remote, err := netip.ParseAddrPort(r.RemoteAddr)
		if err != nil {
			return fmt.Errorf("%w: invalid master remote address: %v", ErrValidation, err)
		}
		masterURL := url.URL{
			Scheme: "http",
			Host:   netip.AddrPortFrom(remote.Addr(), uint16(masterPort)).String(),
		}

		localHash, err := storage.ShowHash(name)
		if err != nil {
			return err
		}
		if localHash != hash {
			masterURL.Path = "/api/shows/" + name + "/data"
			masterURL.RawQuery = "raw=true"
			req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, masterURL.String(), nil)
			if err != nil {
				return err
			}
			client := &http.Client{Timeout: 5 * time.Second}
			resp, err := client.Do(req)
			if err != nil {
				return fmt.Errorf("download show data: %w", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("download show data: master returned %s", resp.Status)
			}
			raw, err := io.ReadAll(resp.Body)
			if err != nil {
				return fmt.Errorf("read downloaded show data: %w", err)
			}
			downloadHash := sha256.Sum256(raw)
			if !bytes.Equal(downloadHash[:], digest) {
				return fmt.Errorf("downloaded show %q does not match the requested sha256", name)
			}
			if _, err := DecodeShow(bytes.NewReader(raw)); err != nil {
				return fmt.Errorf("invalid downloaded show data: %w", err)
			}
			if err := storage.WriteRawShowData(name, bytes.NewReader(raw)); err != nil {
				return fmt.Errorf("save downloaded show data: %w", err)
			}
		}

		player.PlaySlave(name, startTime)
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

		player.Static(state)

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

	get("/api/config", func(w http.ResponseWriter, r *http.Request) error {
		w.Header().Add("Content-Type", "text/toml")

		_, err := w.Write([]byte(config.RawToml()))
		return err
	})

	put("/api/config", func(w http.ResponseWriter, r *http.Request) error {

		body, err := io.ReadAll(r.Body)
		if err != nil {
			return err
		}

		if err := config.WriteToml(string(body)); err != nil {
			return fmt.Errorf("write config: %w", err)
		}

		if err := gpio.Init(config.Pinout(), config.Tracks()); err != nil {
			return fmt.Errorf("re-init gpio: %w", err)
		}

		return nil
	})

	r.Get("/ws", createWebsocketHandler(player))

	staticFS, err := GetStaticFS()
	if err != nil {
		return nil, err
	}
	FileServer(r, "/", staticFS)

	chi.Walk(r, func(method string, route string, handler http.Handler, middlewares ...func(http.Handler) http.Handler) error {
		log.Debug().Str("method", method).Str("route", route).Int("middleware_count", len(middlewares)).Msg("Registered route")
		return nil
	})

	return r, nil
}
