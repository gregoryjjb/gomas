//go:build !gpio

package gpio

import (
	"github.com/stretchr/testify/require"
	"testing"
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
