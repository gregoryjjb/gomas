package main

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"nhooyr.io/websocket"
)

func createWebsocketHandler(player *Player) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			InsecureSkipVerify: true,
		})
		if err != nil {
			http.Error(w, fmt.Sprintf("websocket upgrade failed: %s", err), http.StatusInternalServerError)
			return
		}
		defer c.Close(websocket.StatusInternalError, "the sky is falling")

		// Disabling all this because it's messy and untested

		// unsub, ch := player.Subscribe()
		// defer unsub()

		// for msg := range ch {
		// 	js, err := json.Marshal(msg)
		// 	if err != nil {
		// 		log.Err(err).Msg("Failed to marshal event payload for websocket")
		// 		continue
		// 	}

		// 	if err := writeTimeout(r.Context(), 5*time.Second, c, js); err != nil {
		// 		break
		// 	}
		// }

		// log.Info().Msg("Websocket seems to have closed?")
	}
}

func writeTimeout(ctx context.Context, timeout time.Duration, c *websocket.Conn, msg []byte) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	return c.Write(ctx, websocket.MessageText, msg)
}
