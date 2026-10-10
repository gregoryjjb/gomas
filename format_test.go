package main_test

import (
	"bytes"
	"encoding/json/v2"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	gomas "gregoryjjb/gomas"
)

func fixtureShow(t *testing.T) gomas.ShowData {
	t.Helper()
	f, err := os.Open("testdata/show-v2.json")
	require.NoError(t, err)
	defer f.Close()
	data, err := gomas.DecodeShow(f)
	require.NoError(t, err)
	return data
}

func TestShowRoundTrip(t *testing.T) {
	show := fixtureShow(t)
	raw, err := json.Marshal(show)
	require.NoError(t, err)
	decoded, err := gomas.DecodeShow(bytes.NewReader(raw))
	require.NoError(t, err)
	require.Equal(t, show, decoded)
	root := decoded.Tracks[1].(*gomas.ShowGroup)
	track := root.Children[0].(*gomas.ShowGroup).Children[1].(*gomas.ShowTrack)
	require.True(t, track.Locked)
	require.True(t, track.Keyframes[0].Selected)
}

func TestShowEmptyArrays(t *testing.T) {
	show := gomas.NewShowData([]string{"Roof 1"})
	show.Tracks = append(show.Tracks, &gomas.ShowGroup{Type: gomas.ShowNodeGroup, ID: "Empty"})
	raw, err := json.Marshal(show)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "null")
	require.Contains(t, string(raw), `"keyframes":[]`)
	require.Contains(t, string(raw), `"children":[]`)
	_, err = gomas.DecodeShow(bytes.NewReader(raw))
	require.NoError(t, err)
	raw, err = json.Marshal(gomas.ShowData{Version: "2"})
	require.NoError(t, err)
	require.JSONEq(t, `{"version":"2","tracks":[]}`, string(raw))
}

func TestRejectInvalidShow(t *testing.T) {
	for name, raw := range map[string]string{
		"legacy":            `{"tracks":[]}`,
		"future version":    `{"version":"3","tracks":[]}`,
		"numeric version":   `{"version":2,"tracks":[]}`,
		"missing tracks":    `{"version":"2"}`,
		"null tracks":       `{"version":"2","tracks":null}`,
		"null node":         `{"version":"2","tracks":[null]}`,
		"unknown node":      `{"version":"2","tracks":[{"type":"other","id":"x"}]}`,
		"missing ID":        `{"version":"2","tracks":[{"type":"track","keyframes":[]}]}`,
		"missing keyframes": `{"version":"2","tracks":[{"type":"track","id":"x"}]}`,
		"null keyframes":    `{"version":"2","tracks":[{"type":"track","id":"x","keyframes":null}]}`,
		"invalid locked":    `{"version":"2","tracks":[{"type":"track","id":"x","keyframes":[],"locked":"false"}]}`,
		"invalid selected":  `{"version":"2","tracks":[{"type":"track","id":"x","keyframes":[{"ts":0,"value":1,"selected":1}]}]}`,
		"missing children":  `{"version":"2","tracks":[{"type":"group","id":"x"}]}`,
		"null children":     `{"version":"2","tracks":[{"type":"group","id":"x","children":null}]}`,
		"negative time":     `{"version":"2","tracks":[{"type":"track","id":"x","keyframes":[{"ts":-1,"value":1}]}]}`,
		"duplicate ID":      `{"version":"2","tracks":[{"type":"group","id":"x","children":[{"type":"track","id":"x","keyframes":[]}]}]}`,
		"Beat group":        `{"version":"2","tracks":[{"type":"group","id":"Beat","children":[]}]}`,
		"duplicate field":   `{"version":"2","version":"2","tracks":[]}`,
		"trailing JSON":     `{"version":"2","tracks":[]} {}`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := gomas.DecodeShow(strings.NewReader(raw))
			require.ErrorIs(t, err, gomas.ErrValidation)
		})
	}
}

// Exercise local testing shows too when present, without making CI depend on
// ignored music files. This only decodes and validates them; it does not play audio.
func TestLocalV2Shows(t *testing.T) {
	paths, err := filepath.Glob("data/songs/*/data.json")
	require.NoError(t, err)
	if len(paths) == 0 {
		t.Skip("no local shows")
	}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			f, err := os.Open(path)
			require.NoError(t, err)
			defer f.Close()
			_, err = gomas.DecodeShow(f)
			require.NoError(t, err)
		})
	}
}

func TestShowOptionalBooleans(t *testing.T) {
	for _, value := range []string{"missing", "false", "true", "null"} {
		t.Run(value, func(t *testing.T) {
			locked, selected := "", ""
			if value != "missing" {
				locked = `,"locked":` + value
				selected = `,"selected":` + value
			}
			raw := `{"version":"2","tracks":[{"type":"track","id":"A"` + locked + `,"keyframes":[{"ts":0,"value":1` + selected + `}]}]}`
			show, err := gomas.DecodeShow(strings.NewReader(raw))
			require.NoError(t, err)
			track := show.Tracks[0].(*gomas.ShowTrack)
			require.Equal(t, value == "true", track.Locked)
			require.Equal(t, value == "true", track.Keyframes[0].Selected)
			encoded, err := json.Marshal(show)
			require.NoError(t, err)
			if value == "true" {
				require.Contains(t, string(encoded), `"locked":true`)
				require.Contains(t, string(encoded), `"selected":true`)
			} else {
				require.NotContains(t, string(encoded), `"locked"`)
				require.NotContains(t, string(encoded), `"selected"`)
			}
		})
	}
}

func TestShowIgnoresUnknownFields(t *testing.T) {
	raw := `{
		"version":"2", "editor":{"zoom":2},
		"tracks":[{"type":"group","id":"Root","collapsed":true,"children":[
			{"type":"track","id":"A","color":"red","keyframes":[
				{"ts":1,"value":1,"easing":"linear"}
			]}
		]}]
	}`
	show, err := gomas.DecodeShow(strings.NewReader(raw))
	require.NoError(t, err)
	group := show.Tracks[0].(*gomas.ShowGroup)
	track := group.Children[0].(*gomas.ShowTrack)
	require.Equal(t, "A", track.ID)
	require.Equal(t, []gomas.ShowKeyframe{{Timestamp: 1, Value: 1}}, track.Keyframes)
}

func TestShowKeyframeDefaults(t *testing.T) {
	raw := `{"version":"2","tracks":[{"type":"track","id":"A","keyframes":[
		{}, {"ts":null,"value":null}, {"value":1}, {"ts":2}
	]}]}`
	show, err := gomas.DecodeShow(strings.NewReader(raw))
	require.NoError(t, err)
	track := show.Tracks[0].(*gomas.ShowTrack)
	require.Equal(t, []gomas.ShowKeyframe{{}, {}, {Value: 1}, {Timestamp: 2}}, track.Keyframes)
}

func TestShowKeyframeOrdering(t *testing.T) {
	for _, tc := range []struct {
		name       string
		timestamps []float64
		invalid    bool
	}{
		{"empty", nil, false},
		{"zero", []float64{0}, false},
		{"ascending", []float64{0, 1, 2}, false},
		{"equal", []float64{0, 1, 1}, false},
		{"descending", []float64{2, 1}, true},
		{"middle inversion", []float64{0, 2, 1, 3}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			track := &gomas.ShowTrack{Type: gomas.ShowNodeTrack, ID: "A"}
			for _, ts := range tc.timestamps {
				track.Keyframes = append(track.Keyframes, gomas.ShowKeyframe{Timestamp: ts})
			}
			show := gomas.ShowData{Version: "2", Tracks: gomas.ShowNodes{
				&gomas.ShowGroup{Type: gomas.ShowNodeGroup, ID: "Root", Children: gomas.ShowNodes{track}},
			}}
			err := show.Validate()
			if tc.invalid {
				require.ErrorContains(t, err, "ascending order")
			} else {
				require.NoError(t, err)
			}
			raw, err := json.Marshal(show)
			require.NoError(t, err)
			_, err = gomas.DecodeShow(bytes.NewReader(raw))
			if tc.invalid {
				require.ErrorIs(t, err, gomas.ErrValidation)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
