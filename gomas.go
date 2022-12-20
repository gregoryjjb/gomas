package main

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/log"

	"gregoryjjb/gomas/gpio"
)

func PanicOnError(err error) {
	if err != nil {
		panic(err)
	}
}

func init() {
	InitializeLogger()
}

func main() {
	log.Print("Welcome to Gomas!")

	if err := gpio.Init(); err != nil {
		log.Err(err).Msg("GPIO initialization failed")
	}

	player := NewPlayer()

	StartServer(player)
}

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

func RespondText(w http.ResponseWriter, body string) {
	w.Write([]byte(body))
}

func RespondJSON(w http.ResponseWriter, body any) {
	w.Header().Add("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(body); err != nil {
		RespondInternalServiceError(w, err)
	}
}

func StartServer(player *Player) error {
	r := chi.NewRouter()
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		RespondText(w, "Merry Gomas")
	})

	r.Get("/shows", func(w http.ResponseWriter, r *http.Request) {
		shows, err := ListShows()
		if err != nil {
			RespondInternalServiceError(w, err)
			return
		}

		RespondJSON(w, shows)
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
