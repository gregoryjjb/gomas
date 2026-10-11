package main_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json/v2"
	"errors"
	gomas "gregoryjjb/gomas"
	"maps"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

			player := gomas.NewPlayer(ctx, config, storage, mockAudioPlayer{}, mockGPIO{}, &gomas.SlaveRegistry{})

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

			player := gomas.NewPlayer(t.Context(), config, storage, mockAudioPlayer{}, gpio, &gomas.SlaveRegistry{})

			player.Play("my show")

			time.Sleep(time.Millisecond * 100)

			assert.Equal(t, "playing", player.State())

			time.Sleep(2200 * time.Millisecond)
			assert.Equal(t, "playing", player.State())

			time.Sleep(time.Second)
			assert.Equal(t, "idle", player.State())

			frames, allStates := gpio.takeUpdates()
			assert.Equal(t, []map[string]bool{
				{"Roof 1": true},
				{"Roof 1": false, "Roof 2": true},
				{"Roof 2": false},
			}, frames)
			assert.Equal(t, []bool{false, true}, allStates)

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

func (f fakeStorage) ShowHash(name string) (string, error) {
	show, err := f.ReadShowData(name)
	if err != nil {
		return "", err
	}
	raw, err := json.Marshal(show)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(raw)
	return hex.EncodeToString(hash[:]), nil
}

func (f fakeStorage) ReadAudio(name string) (afero.File, error) {
	return nil, nil
}

func (f fakeStorage) ListShows() ([]string, error) {
	return nil, nil
}

type collectGPIO struct {
	mu        sync.Mutex
	collected []map[string]bool
	setAll    []bool
}

func (c *collectGPIO) Execute(states map[string]bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.collected = append(c.collected, maps.Clone(states))
}

func (c *collectGPIO) SetAll(state bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.setAll = append(c.setAll, state)
}

// takeUpdates drains frame updates and snapshots the cumulative SetAll calls.
func (c *collectGPIO) takeUpdates() ([]map[string]bool, []bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	frames := c.collected
	c.collected = nil
	return frames, append([]bool(nil), c.setAll...)
}

func TestPlayerReadsPinoutFromConfig(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		config := newTestConfig(t, gomas.Flags{}, nil, `data_dir = "/data"
[pinout]
"Roof 1" = 17`)
		gpio := &collectGPIO{}
		player := gomas.NewPlayer(t.Context(), config, fakeStorage{"test": fixtureShow(t)}, mockAudioPlayer{}, gpio, &gomas.SlaveRegistry{})
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
		frames, allStates := gpio.takeUpdates()
		assert.Equal(t, []map[string]bool{{"Roof 2": true}}, frames)
		assert.Equal(t, []bool{false}, allStates)
	})
}

func TestPlayerCatchUp(t *testing.T) {
	type step struct {
		advance time.Duration
		frame   map[string]bool // nil means no GPIO update during this interval
		state   string
	}

	for _, tc := range []struct {
		name       string
		startedAgo time.Duration
		bias       string
		tracks     map[string][]gomas.ShowKeyframe
		steps      []step
	}{
		{
			name:       "LateSlaveCatchesUpEachTrack",
			startedAgo: 5 * time.Second,
			bias:       "0s",
			tracks: map[string][]gomas.ShowKeyframe{
				"Roof 1": {
					{Timestamp: 0, Value: 1}, {Timestamp: 2, Value: 0},
					{Timestamp: 3, Value: 1}, {Timestamp: 7, Value: 0},
				},
				"Roof 2": {
					{Timestamp: 0, Value: 0}, {Timestamp: 4, Value: 1},
					{Timestamp: 5, Value: 0},
				},
				// Roof 3 is empty. Unmapped events must not reach GPIO.
				"Unmapped": {{Timestamp: 0, Value: 1}, {Timestamp: 10, Value: 0}},
			},
			steps: []step{
				{100 * time.Millisecond, map[string]bool{"Roof 1": true, "Roof 2": false}, "playing"},
				{1800 * time.Millisecond, nil, "playing"}, // 6.9s: future event remains pending
				{100 * time.Millisecond, map[string]bool{"Roof 1": false}, "playing"},
				{100 * time.Millisecond, nil, "playing"}, // exhausted tracks must not repeat
			},
		},
		{
			name: "MultipleEventsBetweenTicks",
			bias: "0s",
			tracks: map[string][]gomas.ShowKeyframe{
				"Roof 1": {
					{Timestamp: 0.01, Value: 1}, {Timestamp: 0.02, Value: 0},
					{Timestamp: 0.03, Value: 1}, {Timestamp: 0.09, Value: 0},
				},
			},
			steps: []step{
				{100 * time.Millisecond, map[string]bool{"Roof 1": false}, "playing"},
				{100 * time.Millisecond, nil, "playing"},
			},
		},
		{
			name: "LastEventWinsAtEqualTimestamps",
			bias: "0s",
			tracks: map[string][]gomas.ShowKeyframe{
				"Roof 1": {
					{Timestamp: 0.1, Value: 1}, {Timestamp: 0.1, Value: 0},
					{Timestamp: 0.1, Value: 1}, {Timestamp: 0.3, Value: 0},
				},
			},
			steps: []step{
				{100 * time.Millisecond, map[string]bool{"Roof 1": true}, "playing"},
				{100 * time.Millisecond, nil, "playing"},
				{100 * time.Millisecond, map[string]bool{"Roof 1": false}, "playing"},
				{100 * time.Millisecond, nil, "playing"},
			},
		},
		{
			name: "BiasDelaysCatchUp",
			bias: "200ms",
			tracks: map[string][]gomas.ShowKeyframe{
				"Roof 1": {
					{Timestamp: 0, Value: 1}, {Timestamp: 0.025, Value: 0},
					{Timestamp: 0.05, Value: 1},
				},
			},
			steps: []step{
				{100 * time.Millisecond, nil, "playing"}, // before the biased timeline starts
				{100 * time.Millisecond, map[string]bool{"Roof 1": true}, "playing"},
				{100 * time.Millisecond, map[string]bool{"Roof 1": true}, "playing"},
				{100 * time.Millisecond, nil, "playing"},
			},
		},
		{
			name:       "AppliesFinalStateWhenAlreadyPastEnd",
			startedAgo: 3 * time.Second,
			bias:       "0s",
			tracks: map[string][]gomas.ShowKeyframe{
				"Roof 1": {
					{Timestamp: 0, Value: 1}, {Timestamp: 0.2, Value: 0},
					{Timestamp: 0.4, Value: 1}, {Timestamp: 0.6, Value: 0},
				},
			},
			steps: []step{
				{100 * time.Millisecond, map[string]bool{"Roof 1": false}, "idle"},
				{100 * time.Millisecond, nil, "idle"},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				// A 100ms tick lets us place multiple events between ticks and
				// assert exact boundary behavior without real-time sleeps.
				config := newTestConfig(t, gomas.Flags{}, nil,
					"data_dir = \"/data\"\nframes_per_second = 10\nbias = \""+tc.bias+
						"\"\n[pinout]\n\"Roof 1\" = 17\n\"Roof 2\" = 27\n\"Roof 3\" = 22")
				show := gomas.NewShowData([]string{"Roof 1", "Roof 2", "Roof 3", "Unmapped"})
				for _, node := range show.Tracks {
					track := node.(*gomas.ShowTrack)
					track.Keyframes = tc.tracks[track.ID]
				}
				require.NoError(t, show.Validate())

				gpio := &collectGPIO{}
				player := gomas.NewPlayer(t.Context(), config, fakeStorage{"test": show}, mockAudioPlayer{}, gpio, &gomas.SlaveRegistry{})
				synctest.Wait() // ensure the ticker is initialized before sending the command
				if tc.startedAgo > 0 {
					player.PlaySlave("test", time.Now().Add(-tc.startedAgo))
				} else {
					player.Play("test")
				}
				synctest.Wait()
				require.Equal(t, "playing", player.State())
				frames, allStates := gpio.takeUpdates()
				require.Equal(t, []bool{false}, allStates)
				require.Empty(t, frames)

				for i, step := range tc.steps {
					time.Sleep(step.advance)
					synctest.Wait() // synchronize before inspecting the GPIO mock
					require.Equal(t, step.state, player.State(), "step %d", i)
					frames, allStates = gpio.takeUpdates()
					if step.frame == nil {
						require.Empty(t, frames, "step %d", i)
					} else {
						require.Equal(t, []map[string]bool{step.frame}, frames, "step %d", i)
					}
				}
				if tc.steps[len(tc.steps)-1].state == "idle" {
					require.Equal(t, []bool{false, true}, allStates)
				} else {
					require.Equal(t, []bool{false}, allStates)
				}
			})
		})
	}
}
