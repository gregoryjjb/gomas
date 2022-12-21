//go:build !nogpio

package gpio

import "github.com/stianeikeland/go-rpio/v4"

var pinout []int

var pins []rpio.Pin

func Init(providedPinout []int) error {
	if err := rpio.Open(); err != nil {
		return err
	}

	pinout = providedPinout

	pins = make([]rpio.Pin, 0, len(pinout))
	for _, p := range pinout {
		pin := rpio.Pin(p)
		pin.Output()
		pin.Low()
		pins = append(pins, pin)
	}

	return nil
}

func Close() error {
	return rpio.Close()
}

func Execute(states []bool) error {
	for i, state := range states {
		if i >= len(pins) {
			break
		}

		if state {
			pins[i].High()
		} else {
			pins[i].Low()
		}
	}

	return nil
}

func SetAll(state bool) error {
	states := make([]bool, len(pins))
	for i := range states {
		states[i] = state
	}
	return Execute(states)
}
