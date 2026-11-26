//go:build !gpio

package gpio

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSimulatedTrackMapping(t *testing.T) {
	require.NoError(t, Init([]int{2, 3, 4, 17}, []string{"Roof 1", "Roof 2", "Roof 1"}))
	require.Equal(t, map[int]bool{2: false, 3: false, 4: false, 17: false}, currentState)
	require.NoError(t, Execute(map[string]bool{"Roof 2": true, "unknown": true}))
	require.Equal(t, map[int]bool{2: false, 3: true, 4: false, 17: false}, currentState)
	require.NoError(t, Execute(map[string]bool{"Roof 1": true}))
	require.Equal(t, map[int]bool{2: true, 3: true, 4: true, 17: false}, currentState)
	require.NoError(t, Execute(map[string]bool{"Roof 1": false}))
	require.Equal(t, map[int]bool{2: false, 3: true, 4: false, 17: false}, currentState)
	require.NoError(t, SetAll(true))
	require.Equal(t, map[int]bool{2: true, 3: true, 4: true, 17: true}, currentState)
	require.NoError(t, SetAll(false))
	require.Equal(t, map[int]bool{2: false, 3: false, 4: false, 17: false}, currentState)

	// Reinitialization must not retain any old mapping or pin state.
	require.NoError(t, Init([]int{22}, []string{"Fence"}))
	require.NoError(t, Execute(map[string]bool{"Roof 1": true}))
	require.Equal(t, map[int]bool{22: false}, currentState)
	require.NoError(t, Init(nil, nil))
	require.NoError(t, Execute(map[string]bool{"Fence": true}))
	require.NoError(t, SetAll(true))
	require.Empty(t, currentState)
	require.Error(t, Init([]int{2}, []string{"A", "A"}))
}

func TestConcurrentSimulatedReinit(t *testing.T) {
	require.NoError(t, Init([]int{2, 3}, []string{"Roof", "Roof"}))
	t.Cleanup(func() { require.NoError(t, Init(nil, nil)) })

	start := make(chan struct{})
	var workers sync.WaitGroup
	for _, operation := range []func() error{
		func() error { return Init([]int{2, 3}, []string{"Roof", "Roof"}) },
		func() error { return Init([]int{17}, []string{"Fence"}) },
		func() error { return Init(nil, nil) },
		func() error { return Execute(map[string]bool{"Roof": true, "Fence": false}) },
		func() error { return SetAll(true) },
		func() error { return SetAll(false) },
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

	// After contention, the latest mapping must remain fully usable.
	require.NoError(t, Init([]int{22, 26}, []string{"Roof", "Roof"}))
	require.NoError(t, Execute(map[string]bool{"Roof": true}))
	require.Equal(t, map[int]bool{22: true, 26: true}, currentState)
	// A rejected reconfiguration must preserve the active mapping and state.
	require.Error(t, Init([]int{2}, []string{"A", "B"}))
	require.Equal(t, map[int]bool{22: true, 26: true}, currentState)
	require.NoError(t, SetAll(false))
	require.Equal(t, map[int]bool{22: false, 26: false}, currentState)
}
