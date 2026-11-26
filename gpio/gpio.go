//go:build gpio

package gpio

import (
	"sync"

	"github.com/rs/zerolog/log"
	"github.com/stianeikeland/go-rpio/v4"
)

var pins []rpio.Pin
var trackPins map[string][]int
var mu sync.Mutex
var opened bool

func Init(pinout []int, tracks []string) error {
	mu.Lock()
	defer mu.Unlock()

	mapping, err := mapTrackPins(pinout, tracks)
	if err != nil {
		return err
	}
	if len(pinout) > 0 && !opened {
		if err := rpio.Open(); err != nil {
			return err
		}
		opened = true
	}
	// Removed pins must not remain on after a configuration change.
	for _, pin := range pins {
		pin.Low()
	}
	pins = make([]rpio.Pin, len(pinout))
	for i, number := range pinout {
		pin := rpio.Pin(number)
		pin.Output()
		pin.Low()
		pins[i] = pin
	}
	trackPins = mapping
	if len(pinout) == 0 {
		log.Info().Msg("No GPIO pinout configured; skipping hardware initialization")
	}
	return nil
}

func Close() error {
	mu.Lock()
	defer mu.Unlock()

	if !opened {
		return nil
	}
	// Do not let later calls access unmapped registers, even if Close fails.
	pins = nil
	trackPins = nil
	opened = false
	return rpio.Close()
}

func Execute(states map[string]bool) error {
	mu.Lock()
	defer mu.Unlock()

	for name, state := range states {
		for _, number := range trackPins[name] {
			pin := rpio.Pin(number)
			if state {
				pin.High()
			} else {
				pin.Low()
			}
		}
	}
	return nil
}

func SetAll(state bool) error {
	mu.Lock()
	defer mu.Unlock()

	for _, pin := range pins {
		if state {
			pin.High()
		} else {
			pin.Low()
		}
	}
	return nil
}
