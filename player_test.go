package main_test

import (
	"context"
	gomas "gregoryjjb/gomas"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestPlayer(t *testing.T) {

	t.Run("StopsWhenContextCancelled", func(t *testing.T) {
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

		ctx, cancel := context.WithCancel(context.Background())
		storage := gomas.NewStorage(fs, config)

		player := gomas.NewPlayer(ctx, config, storage, mockAudioPlayer{})

		assert.Equal(t, "idle", player.State())

		cancel()

		// Hack: give the player loop time to catch the context cancellation
		time.Sleep(time.Millisecond * 100)

		assert.Equal(t, "dead", player.State())
	})
}
