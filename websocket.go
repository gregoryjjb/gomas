package main

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/rs/zerolog/log"
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

		// player.Subscribe(func(s string) {
		// 	if err := c.Write(r.Context(), websocket.MessageText, []byte(s)); err != nil {
		// 		log.Err(err).Msg("Websocket write failed")
		// 	}
		// })
		
		unsub, ch := player.Subscribe()
		defer unsub()

		for msg := range ch {
			fmt.Println("PULLED MSG FROM CHANNEL ", msg)
			// if err := c.Write(r.Context(), websocket.MessageText, []byte(msg)); err != nil {
			// 	log.Err(err).Msg("Websocket write failed")
			// }

			if err := writeTimeout(r.Context(), 5*time.Second, c, []byte(msg)); err != nil {
				break
			}
		}

		log.Info().Msg("Websocket seems to have closed?")
		

		// var v interface{}
		// err = wsjson.Read(r.Context(), c, &v)
		// if err != nil {
		// 	// ...
		// }

		// log.Printf("received: %v", v)

		// c.Close(websocket.StatusNormalClosure, "")
	}
}

func writeTimeout(ctx context.Context, timeout time.Duration, c *websocket.Conn, msg []byte) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	return c.Write(ctx, websocket.MessageText, msg)
}
