//go:build !nogpio

package main

import "github.com/stianeikeland/go-rpio/v4"

var pinout = []int{4, 17, 27, 22, 5, 6, 13, 26}

type GPIO struct {
	pins []rpio.Pin
}

func NewGPIO() (*GPIO, error) {
	if err := rpio.Open(); err != nil {
		return nil, err
	}

	pins := make([]rpio.Pin, 0, len(pinout))
	for _, p := range pinout {
		pin := rpio.Pin(p)
		pin.Output()
		pin.Low()
		pins = append(pins, pin)
	}

	return &GPIO{
		pins: pins,
	}, nil
}

func (g *GPIO) Execute(states []bool) error {
	for i, state := range states {
		if i >= len(g.pins) {
			break
		}

		if state {
			g.pins[i].High()
		} else {
			g.pins[i].Low()
		}
	}

	return nil
}
