//go:build !gpio

package gpio

import (
	"github.com/rs/zerolog/log"
)

func Init(providedPinout []int) error {
	log.Debug().Msg("GPIO will be simulated")
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

func Execute(states []bool) error {
	printStates(states)
	return nil
}

func SetAll(state bool) error {
	states := make([]bool, 8)
	for i := range states {
		states[i] = state
	}
	printStates(states)
	return nil
}
