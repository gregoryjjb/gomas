package main_test

import (
	"context"
	"errors"
	gomas "gregoryjjb/gomas"
	"maps"
	"testing"
	"testing/synctest"
	"time"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
)

type mockGPIO struct{}

func (mockGPIO) Execute(map[string]bool) {}
func (mockGPIO) SetAll(bool)             {}

func TestPlayer(t *testing.T) {
	config := newTestConfig(t,
		gomas.Flags{},
		map[string]string{
			"HOST": "127.0.0.1",
			"PORT": "1225",
		},
		`data_dir = "/data"
[pinout]
"Roof 1" = 17
"Roof 2" = 27`,
	)

	t.Run("StopsWhenContextCancelled", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			fs := gomas.NewGomasMemFS()
			fs.MkdirAll("/data/projects", 0755)

			ctx, cancel := context.WithCancel(context.Background())
			storage := gomas.NewStorage(fs, config)

			player := gomas.NewPlayer(ctx, config, storage, mockAudioPlayer{}, mockGPIO{})

			assert.Equal(t, "idle", player.State())

			cancel()

			synctest.Wait()

			assert.Equal(t, "dead", player.State())
		})

	})

	t.Run("PlaysKeyframes", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			// Lighting ends at 2s on an unmapped track; the Beat marker at 100s
			// must not extend playback.
			storage := fakeStorage{"my show": fixtureShow(t)}

			gpio := &collectGPIO{}

			player := gomas.NewPlayer(t.Context(), config, storage, mockAudioPlayer{}, gpio)

			player.Play("my show")

			time.Sleep(time.Millisecond * 100)

			assert.Equal(t, "playing", player.State())

			time.Sleep(2200 * time.Millisecond)
			assert.Equal(t, "playing", player.State())

			time.Sleep(time.Second)
			assert.Equal(t, "idle", player.State())

			assert.Equal(t, []map[string]bool{
				{"Roof 1": true},
				{"Roof 1": false, "Roof 2": true},
				{"Roof 2": false},
			}, gpio.collected)
			assert.Equal(t, []bool{false, true}, gpio.setAll)

		})
	})
}

type fakeStorage map[string]gomas.ShowData

func (f fakeStorage) ReadShowData(name string) (gomas.ShowData, error) {
	p, ok := f[name]
	if !ok {
		return gomas.ShowData{}, errors.New("project not found")
	}

	return p, nil
}

func (f fakeStorage) ReadAudio(name string) (afero.File, error) {
	return nil, nil
}

func (f fakeStorage) ListShows() ([]string, error) {
	return nil, nil
}

type collectGPIO struct {
	collected []map[string]bool
	setAll    []bool
}

func (c *collectGPIO) Execute(states map[string]bool) {
	c.collected = append(c.collected, maps.Clone(states))
}

func (c *collectGPIO) SetAll(state bool) {
	c.setAll = append(c.setAll, state)
}

func TestPlayerReadsPinoutFromConfig(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		config := newTestConfig(t, gomas.Flags{}, nil, `data_dir = "/data"
[pinout]
"Roof 1" = 17`)
		gpio := &collectGPIO{}
		player := gomas.NewPlayer(t.Context(), config, fakeStorage{"test": fixtureShow(t)}, mockAudioPlayer{}, gpio)
		// Reassign the same physical pin before loading the show. The player
		// should resolve the current config, not names saved in its constructor.
		err := config.WriteToml(`data_dir = "/data"
[pinout]
"Roof 2" = 17`)
		if !assert.NoError(t, err) {
			return
		}
		player.Play("test")
		time.Sleep(610 * time.Millisecond)
		assert.Equal(t, []map[string]bool{{"Roof 2": true}}, gpio.collected)
		assert.Equal(t, []bool{false}, gpio.setAll)
	})
}
