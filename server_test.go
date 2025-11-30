package main_test

import (
	"context"
	"fmt"
	gomas "gregoryjjb/gomas"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/spf13/afero"
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

func newTestConfig(t *testing.T, flags gomas.Flags, env map[string]string, toml string) *gomas.Config {
	fs := gomas.NewGomasMemFS()

	require.NoError(t, fs.Mkdir("/data", 0777))
	require.NoError(t, afero.WriteFile(fs, "/gomas.toml", []byte(toml), 0777))

	c, err := gomas.NewConfig(fs, flags, func(s string) string { return env[s] })
	require.NoError(t, err)

	return c
}

func TestStartServer(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	config := newTestConfig(t,
		gomas.Flags{},
		map[string]string{
			"HOST": "127.0.0.1",
			"PORT": "1225",
		},
		`data_dir = "/data"`,
	)

	fs := gomas.NewGomasMemFS()
	fs.MkdirAll("/data/projects", 0755)

	buildInfo := gomas.BuildInfo{
		Version: "0.0.0",
	}
	storage := gomas.NewStorage(fs, config)
	player := gomas.NewPlayer(ctx, config, storage, mockAudioPlayer{}, mockGPIO{})

	go gomas.StartServer(config, buildInfo, player, storage)

	err := waitForReady(ctx, time.Second*600, "http://localhost:1225/api/shows")
	require.NoError(t, err)
}
