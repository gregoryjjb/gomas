//go:build nogpio

package main

type GPIO struct {
}

func NewGPIO() (*GPIO, error) {

	return &GPIO{}, nil
}

func (g *GPIO) Execute(states []bool) error {
	return nil
}
