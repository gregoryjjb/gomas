package main_test

import (
	gomas "gregoryjjb/gomas"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestProjectData_FlatKeyframes(t *testing.T) {
	tests := []struct {
		name string // description of this test case
		// Named input parameters for receiver constructor.
		in   gomas.ProjectData
		want []gomas.FlatKeyframe
	}{
		// TODO: Add test cases.
		{
			name: "success",
			in: gomas.ProjectData{
				Tracks: []gomas.Track{
					{
						Name: "0",
						Keyframes: []gomas.Keyframe{
							{
								Timestamp: 0,
								Value:     0,
							},
							{
								Timestamp: 0.5,
								Value:     1,
							},
						},
					},
				},
			},
			want: []gomas.FlatKeyframe{{
				Time:   0,
				States: []bool{false},
			}, {
				Time:   0.5,
				States: []bool{true},
			}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.in.FlatKeyframes()
			assert.Equal(t, tt.want, got)
		})
	}
}
