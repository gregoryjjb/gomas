//go:build gpio

package gpio_test

import (
	"sync"
	"testing"

	"gregoryjjb/gomas/gpio"
)

func TestEmptyPinout(t *testing.T) {
	for _, tc := range []struct {
		name   string
		pinout []int
	}{
		{"omitted", nil},
		{"empty", []int{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := gpio.Init(tc.pinout, nil); err != nil {
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

// Empty pinouts exercise the hardware backend's synchronization without
// requiring access to a Raspberry Pi's memory-mapped registers.
func TestConcurrentEmptyPinout(t *testing.T) {
	start := make(chan struct{})
	var workers sync.WaitGroup
	for _, operation := range []func() error{
		func() error { return gpio.Init(nil, nil) },
		func() error { return gpio.Execute(map[string]bool{"unmapped": true}) },
		func() error { return gpio.SetAll(false) },
		gpio.Close,
	} {
		workers.Go(func() {
			<-start
			for range 100 {
				if err := operation(); err != nil {
					t.Error(err)
				}
			}
		})
	}
	close(start)
	workers.Wait()
}
