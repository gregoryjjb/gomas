//go:build gpio

package gpio

import (
	"github.com/rs/zerolog/log"
	"github.com/stianeikeland/go-rpio/v4"
)

var pins []rpio.Pin
var trackPins map[string][]int

func Init(pinout []int, tracks []string) error {
	mapping, err := mapTrackPins(pinout, tracks)
	if err != nil {
		return err
	}
	if len(pinout) == 0 {
		pins = nil
		trackPins = mapping
		log.Info().Msg("No GPIO pinout configured; skipping hardware initialization")
		return nil
	}
	if err := rpio.Open(); err != nil {
		return err
	}
	pins = make([]rpio.Pin, len(pinout))
	for i, number := range pinout {
		pin := rpio.Pin(number)
		pin.Output()
		pin.Low()
		pins[i] = pin
	}
	trackPins = mapping
	return nil
}

func Close() error {
	if len(pins) == 0 {
		return nil
	}
	return rpio.Close()
}

func Execute(states map[string]bool) error {
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
	for _, pin := range pins {
		if state {
			pin.High()
		} else {
			pin.Low()
		}
	}
	return nil
}
