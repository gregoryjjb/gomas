//go:build gpio

package gpio

import (
	"github.com/rs/zerolog/log"
	"github.com/stianeikeland/go-rpio/v4"
)

var pins map[string]rpio.Pin

func Init(pinout map[string]int) error {
	if len(pinout) == 0 {
		log.Info().Msg("No GPIO pinout configured; skipping hardware initialization")
		return nil
	}

	if err := rpio.Open(); err != nil {
		return err
	}

	pins = make(map[string]rpio.Pin, len(pinout))
	for name, number := range pinout {
		pin := rpio.Pin(number)
		pin.Output()
		pin.Low()
		pins[name] = pin
	}

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
		if pin, ok := pins[name]; ok {
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
	states := make(map[string]bool, len(pins))
	for name := range pins {
		states[name] = state
	}
	return Execute(states)
}
