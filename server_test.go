package main_test

import (
	"context"
	gomas "gregoryjjb/gomas"
	"io"
	"net/http/httptest"
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

func newTestConfig(t *testing.T, flags gomas.Flags, env map[string]string, toml string) *gomas.Config {
	fs := gomas.NewGomasMemFS()

	require.NoError(t, fs.Mkdir("/data", 0777))
	require.NoError(t, afero.WriteFile(fs, "/gomas.toml", []byte(toml), 0777))

	c, err := gomas.NewConfig(fs, flags, func(s string) string { return env[s] })
	require.NoError(t, err)

	return c
}

func TestServer(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	config := newTestConfig(t,
		gomas.Flags{},
		nil,
		`data_dir = "/data"
[pinout]
"Roof 1" = 17`,
	)

	fs := gomas.NewGomasMemFS()
	require.NoError(t, fs.MkdirAll("/data/songs", 0755))

	buildInfo := gomas.BuildInfo{
		Version: "0.0.0",
	}
	storage := gomas.NewStorage(fs, config)
	player := gomas.NewPlayer(ctx, config, storage, mockAudioPlayer{}, mockGPIO{}, &gomas.SlaveRegistry{})

	handler, err := gomas.NewServerHandler(config, buildInfo, player, storage)
	require.NoError(t, err)
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	testShowAPI(t, storage, server.URL)
}

func TestServerCancelledContext(t *testing.T) {
	config := newTestConfig(t, gomas.Flags{}, map[string]string{"PORT": "0"}, "data_dir = \"/data\"")
	fs := gomas.NewGomasMemFS()
	storage := gomas.NewStorage(fs, config)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	// A cancelled startup must release its randomly allocated HTTP listener.
	err := gomas.StartServer(ctx, config, gomas.BuildInfo{}, nil, storage, &gomas.SlaveRegistry{})
	require.ErrorIs(t, err, context.Canceled)
}
