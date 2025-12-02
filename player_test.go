package main_test

import (
	"context"
	"errors"
	"fmt"
	gomas "gregoryjjb/gomas"
	"testing"
	"time"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
)

type mockGPIO struct{}

func (mockGPIO) Execute([]bool) {}
func (mockGPIO) SetAll(bool)    {}

func TestPlayer(t *testing.T) {
	config := newTestConfig(t,
		gomas.Flags{},
		map[string]string{
			"HOST": "127.0.0.1",
			"PORT": "1225",
		},
		`data_dir = "/data"`,
	)

	t.Run("StopsWhenContextCancelled", func(t *testing.T) {
		fs := gomas.NewGomasMemFS()
		fs.MkdirAll("/data/projects", 0755)

		ctx, cancel := context.WithCancel(context.Background())
		storage := gomas.NewStorage(fs, config)

		player := gomas.NewPlayer(ctx, config, storage, mockAudioPlayer{}, mockGPIO{})

		assert.Equal(t, "idle", player.State())

		cancel()

		// Hack: give the player loop time to catch the context cancellation
		time.Sleep(time.Millisecond * 100)

		assert.Equal(t, "dead", player.State())
	})

	t.Run("PlaysKeyframes", func(t *testing.T) {
		storage := fakeStorage(map[string]gomas.ProjectData{
			"my show": gomas.ProjectData{
				Tracks: []gomas.Track{
					{
						Name: "0",
						Keyframes: []gomas.Keyframe{
							{
								Timestamp: 0,
								Value:     0,
							},
							{
								Timestamp: 0.5,
								Value:     1,
							},
						},
					},
				},
			},
		})

		gpio := &collectGPIO{}

		player := gomas.NewPlayer(context.Background(), config, storage, mockAudioPlayer{}, gpio)

		player.Play("my show")

		time.Sleep(time.Millisecond * 100)

		assert.Equal(t, "playing", player.State())

		time.Sleep(time.Second * 3)

		assert.Equal(t, "idle", player.State())

		assert.Equal(t, [][]bool{{false}, {true}}, gpio.collected)
	})
}

type fakeStorage map[string]gomas.ProjectData

func (f fakeStorage) ReadShowData(name string) (gomas.ProjectData, error) {
	p, ok := f[name]
	if !ok {
		return gomas.ProjectData{}, errors.New("project not found")
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
	collected [][]bool
}

func (c *collectGPIO) Execute(states []bool) {
	fmt.Println("GPIO", states)
	c.collected = append(c.collected, states)
}

func (c *collectGPIO) SetAll(bool) {}
