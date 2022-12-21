package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
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

//go:embed www/index.html
var indexTemplateEmbed string

func StartServer(player *Player) error {
	indexTemplate, err := template.New("index.html").Parse(indexTemplateEmbed)
	if err != nil {
		return err
	}

	r := chi.NewRouter()
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		shows, err := ListShows()
		if err != nil {
			RespondInternalServiceError(w, err)
			return
		}

		indexTemplate.Execute(w, shows)
	})

	// Cram the legacy show editor in
	FileServer(r, "/editor", http.Dir("www/editor"))

	// GET shows
	r.Get("/shows", func(w http.ResponseWriter, r *http.Request) {
		shows, err := ListShows()
		if err != nil {
			RespondInternalServiceError(w, err)
			return
		}

		RespondJSON(w, shows)
	})

	// GET single show
	r.Get("/shows/{id}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Cache-Control", "no-cache, no-store")
		id := chi.URLParam(r, "id")
		show, err := LoadShow(id)
		if err != nil {
			RespondInternalServiceError(w, err)
			return
		}

		RespondJSON(w, show)
	})

	// POST single show
	r.Post("/shows/{id}", func(w http.ResponseWriter, r *http.Request) {
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

		if len(show.Tracks) == 0 {
			RespondBadRequest(w, "show had zero tracks, refusing to save!")
		}

		if err := SaveShow(id, &show); err != nil {
			RespondInternalServiceError(w, err)
		}

		w.WriteHeader(http.StatusNoContent)
	})

	// GET audio file for single show
	r.Get("/audio/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		audioPath, err := ShowAudioPath(id)
		if err != nil {
			RespondNotFoundError(w, "audio not found")
			return
		}
		http.ServeFile(w, r, audioPath)
	})

	r.Get("/play/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		exists, err := ShowExists(id)
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

	r.Get("/playall", func(w http.ResponseWriter, r *http.Request) {
		player.PlayAll()
		w.WriteHeader(http.StatusNoContent)
	})

	r.Get("/static", func(w http.ResponseWriter, r *http.Request) {
		player.Stop()
		w.WriteHeader(http.StatusNoContent)
	})

	r.Get("/next", func(w http.ResponseWriter, r *http.Request) {
		player.Next()
		w.WriteHeader(http.StatusNoContent)
	})

	address := fmt.Sprintf("%s:%s", Host, Port)
	log.Info().Str("listen", address).Msg("launching server")
	return http.ListenAndServe(address, r)
}
