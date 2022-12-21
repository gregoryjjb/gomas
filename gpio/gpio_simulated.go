//go:build nogpio

package gpio

import "fmt"

const prefix = "Simulated GPIO: "

func Init(providedPinout []int) error {
	fmt.Println(prefix, "initializing")
	return nil
}

func Close() error {
	fmt.Println(prefix, "closing")
	return nil
}

func printStates(states []bool) {
	str := prefix
	for _, state := range states {
		if state {
			str += "#"
		} else {
			str += " "
		}
	}
	fmt.Println(str)
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
