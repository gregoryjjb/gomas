//go:build gpio

package gpio_test

import (
	"gregoryjjb/gomas/gpio"
	"testing"
)

func TestEmptyPinout(t *testing.T) {
	for _, tc := range []struct {
		name   string
		pinout map[string]int
	}{
		{"omitted", nil},
		{"empty", map[string]int{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := gpio.Init(tc.pinout); err != nil {
				t.Fatal(err)
			}
			if err := gpio.SetAll(true); err != nil {
				t.Fatal(err)
			}
			if err := gpio.SetAll(false); err != nil {
				t.Fatal(err)
			}
			if err := gpio.Execute(map[string]bool{"unmapped": true}); err != nil {
				t.Fatal(err)
			}
			if err := gpio.Close(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
