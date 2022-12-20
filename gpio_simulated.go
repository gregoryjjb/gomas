//go:build nogpio

package main

import "fmt"

type GPIO struct {
}

func NewGPIO() (*GPIO, error) {

	return &GPIO{}, nil
}

func printStates(states []bool) {
	str := "GPIO: "
	for _, state := range states {
		if state {
			str += "#"
		} else {
			str += " "
		}
	}
	fmt.Println(str)
}

func (g *GPIO) Execute(states []bool) error {
	printStates(states)
	return nil
}

func (g *GPIO) SetAll(state bool) error {
	states := make([]bool, 8)
	for i := range states {
		states[i] = state
	}
	printStates(states)
	return nil
}
