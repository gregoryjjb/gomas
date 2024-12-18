package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type mockAudioPlayer struct{}

func (mockAudioPlayer) Play(r io.ReadCloser) (time.Time, error) {
	return time.Now(), nil
}

func (mockAudioPlayer) Stop()  {}
func (mockAudioPlayer) Close() {}

// waitForReady calls the specified endpoint until it gets a 200
// response or until the context is cancelled or the timeout is
// reached.
func waitForReady(
	ctx context.Context,
	timeout time.Duration,
	endpoint string,
) error {
	client := http.Client{}
	startTime := time.Now()
	for {
		req, err := http.NewRequestWithContext(
			ctx,
			http.MethodGet,
			endpoint,
			nil,
		)
		if err != nil {
			return fmt.Errorf("failed to create request: %w", err)
		}

		resp, err := client.Do(req)
		if err != nil {
			fmt.Printf("Error making request: %s\n", err.Error())
			// continue
		} else {
			if resp.StatusCode == http.StatusOK {
				fmt.Println("Endpoint is ready!")
				resp.Body.Close()
				return nil
			}
			resp.Body.Close()
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			if time.Since(startTime) >= timeout {
				return fmt.Errorf("timeout reached while waiting for endpoint")
			}
			// wait a little while between checks
			time.Sleep(250 * time.Millisecond)
		}
	}
}

func TestStartServer(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	config := NewConfig(ConfigTOML{
		DataDir: "/data",
	})
	config.Host = "127.0.0.1"
	config.Port = "1225"

	fs := NewGomasMemFS()
	fs.MkdirAll("/data/projects", 0755)

	buildInfo := BuildInfo{
		Version: "0.0.0",
	}
	storage := NewStorage(fs, config)
	player := NewPlayer(ctx, config, storage, mockAudioPlayer{})

	go StartServer(config, buildInfo, player, storage)

	err := waitForReady(ctx, time.Second*600, "http://localhost:1225/api/shows")
	require.NoError(t, err)
}
