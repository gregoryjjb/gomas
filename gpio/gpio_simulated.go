//go:build !gpio

package gpio

import (
	"maps"
	"slices"

	"github.com/rs/zerolog/log"
)

func Init(pinout map[string]int) error {
	log.Debug().Msg("GPIO will be simulated")

	for k := range pinout {
		currentState[k] = false
	}

	return nil
}

func Close() error {
	log.Debug().Msg("Simulated GPIO closing")
	return nil
}

func printStates(states []bool) {
	var str string
	for _, state := range states {
		if state {
			str += "#"
		} else {
			str += " "
		}
	}
	log.Debug().Str("pins", str).Msg("GPIO")
}

// func Execute(states map[string]bool) error {
// 	printStates(slices.Collect(maps.Values(states)))
// 	return nil
// }

var currentState = make(map[string]bool)

func printCurrentState() {
	
	keys := slices.Collect(maps.Keys(currentState))
	slices.Sort(keys)

	var str string
	for _, k := range keys {
		if currentState[k] {
			str += "#"
		} else {
			str += " "
		}
	}

	log.Debug().Str("pins", str).Msg("GPIO")
}

func Execute(states map[string]bool) error {
	// printStates(slices.Collect(maps.Values(states)))

	for k, v := range states {
		currentState[k] = v
	}

	printCurrentState()
	return nil
}

func SetAll(state bool) error {
	for k := range currentState {
		currentState[k] = state
	}

	printCurrentState()
	return nil
}
