//go:build !gpio

package gpio

import (
	"slices"

	"github.com/rs/zerolog/log"
)

var pins []int
var trackPins map[string][]int
var currentState map[int]bool

func Init(pinout []int, tracks []string) error {
	mapping, err := mapTrackPins(pinout, tracks)
	if err != nil {
		return err
	}
	log.Debug().Msg("GPIO will be simulated")
	pins = slices.Clone(pinout)
	trackPins = mapping
	currentState = make(map[int]bool, len(pinout))
	for _, pin := range pins {
		currentState[pin] = false
	}
	return nil
}

func Close() error {
	log.Debug().Msg("Simulated GPIO closing")
	return nil
}

func printCurrentState() {
	var str string
	for _, pin := range pins {
		if currentState[pin] {
			str += "#"
		} else {
			str += " "
		}
	}
	log.Debug().Str("pins", str).Msg("GPIO")
}

func Execute(states map[string]bool) error {
	for name, state := range states {
		for _, pin := range trackPins[name] {
			currentState[pin] = state
		}
	}
	printCurrentState()
	return nil
}

func SetAll(state bool) error {
	for _, pin := range pins {
		currentState[pin] = state
	}
	printCurrentState()
	return nil
}
