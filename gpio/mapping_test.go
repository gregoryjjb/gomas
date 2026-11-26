package gpio

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestTrackPinMapping(t *testing.T) {
	for _, tc := range []struct {
		name   string
		pinout []int
		tracks []string
		want   map[string][]int
	}{
		{"ordered", []int{2, 3, 4}, []string{"Roof 1", "Roof 2"}, map[string][]int{"Roof 1": {2}, "Roof 2": {3}}},
		{"reordered pins", []int{3, 2}, []string{"Roof 1", "Roof 2"}, map[string][]int{"Roof 1": {3}, "Roof 2": {2}}},
		{"reordered tracks", []int{2, 3}, []string{"Roof 2", "Roof 1"}, map[string][]int{"Roof 2": {2}, "Roof 1": {3}}},
		{"duplicate tracks", []int{2, 3, 4}, []string{"Roof 1", "Roof 2", "Roof 1"}, map[string][]int{"Roof 1": {2, 4}, "Roof 2": {3}}},
		{"no tracks", []int{2, 3}, nil, map[string][]int{}},
		{"no pins", nil, nil, map[string][]int{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mapping, err := mapTrackPins(tc.pinout, tc.tracks)
			require.NoError(t, err)
			require.Equal(t, tc.want, mapping)
		})
	}
	_, err := mapTrackPins([]int{2}, []string{"Roof 1", "Roof 1"})
	require.Error(t, err)
}
