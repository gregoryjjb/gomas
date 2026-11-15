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
	f, err := os.Open("testdata/show-v3.json")
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
	require.Equal(t, "2026-11-08T12:00:00.000Z", decoded.UpdatedAt)
	track := decoded.Tracks["Roof 1"]
	require.True(t, track.Locked)
	require.True(t, track.Keyframes[0].Selected)
}

func TestShowEmptyCollections(t *testing.T) {
	show := gomas.NewShowData([]string{"Roof 1"})
	raw, err := json.Marshal(show)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "null")
	require.NotContains(t, string(raw), "updatedAt")
	require.Contains(t, string(raw), `"keyframes":[]`)
	require.Contains(t, string(raw), `"locked":false`)
	_, err = gomas.DecodeShow(bytes.NewReader(raw))
	require.NoError(t, err)
	raw, err = json.Marshal(gomas.ShowData{Version: "3"})
	require.NoError(t, err)
	require.JSONEq(t, `{"version":"3","tracks":{}}`, string(raw))
	_, err = gomas.DecodeShow(bytes.NewReader(raw))
	require.NoError(t, err)
}

func TestRejectInvalidShow(t *testing.T) {
	for name, raw := range map[string]string{
		"unversioned":       `{"tracks":[]}`,
		"version 1":         `{"version":"1","tracks":{}}`,
		"version 2 tree":    `{"version":"2","tracks":[{"type":"track","id":"A","keyframes":[]}]}`,
		"version 2 map":     `{"version":"2","tracks":{}}`,
		"future version":    `{"version":"4","tracks":{}}`,
		"numeric version":   `{"version":3,"tracks":{}}`,
		"missing tracks":    `{"version":"3"}`,
		"null tracks":       `{"version":"3","tracks":null}`,
		"array tracks":      `{"version":"3","tracks":[]}`,
		"null track":        `{"version":"3","tracks":{"A":null}}`,
		"empty name":        `{"version":"3","tracks":{"":{"keyframes":[]}}}`,
		"blank name":        `{"version":"3","tracks":{"  ":{"keyframes":[]}}}`,
		"missing keyframes": `{"version":"3","tracks":{"A":{}}}`,
		"null keyframes":    `{"version":"3","tracks":{"A":{"keyframes":null}}}`,
		"invalid locked":    `{"version":"3","tracks":{"A":{"keyframes":[],"locked":"false"}}}`,
		"invalid selected":  `{"version":"3","tracks":{"A":{"keyframes":[{"ts":0,"value":1,"selected":1}]}}}`,
		"negative time":     `{"version":"3","tracks":{"A":{"keyframes":[{"ts":-1,"value":1}]}}}`,
		"duplicate track":   `{"version":"3","tracks":{"A":{"keyframes":[]},"A":{"keyframes":[]}}}`,
		"duplicate field":   `{"version":"3","version":"3","tracks":{}}`,
		"trailing JSON":     `{"version":"3","tracks":{}} {}`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := gomas.DecodeShow(strings.NewReader(raw))
			require.ErrorIs(t, err, gomas.ErrValidation)
		})
	}
}

// Exercise local shows when present without making CI depend on ignored music files.
func TestLocalV3Shows(t *testing.T) {
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
			raw := `{"version":"3","tracks":{"A":{"keyframes":[{"ts":0,"value":1` + selected + `}]` + locked + `}}}`
			show, err := gomas.DecodeShow(strings.NewReader(raw))
			require.NoError(t, err)
			track := show.Tracks["A"]
			require.Equal(t, value == "true", track.Locked)
			require.Equal(t, value == "true", track.Keyframes[0].Selected)
			encoded, err := json.Marshal(show)
			require.NoError(t, err)
			if value == "true" {
				require.Contains(t, string(encoded), `"locked":true`)
				require.Contains(t, string(encoded), `"selected":true`)
			} else {
				require.Contains(t, string(encoded), `"locked":false`)
				require.NotContains(t, string(encoded), `"selected"`)
			}
		})
	}
}

func TestShowIgnoresUnknownFields(t *testing.T) {
	raw := `{"version":"3","editor":{"zoom":2},"tracks":{"A":{"color":"red","keyframes":[{"ts":1,"value":1,"easing":"linear"}]}}}`
	show, err := gomas.DecodeShow(strings.NewReader(raw))
	require.NoError(t, err)
	require.Equal(t, []gomas.ShowKeyframe{{Timestamp: 1, Value: 1}}, show.Tracks["A"].Keyframes)
}

func TestShowKeyframeDefaults(t *testing.T) {
	raw := `{"version":"3","tracks":{"A":{"keyframes":[{}, {"ts":null,"value":null}, {"value":1}, {"ts":2}]}}}`
	show, err := gomas.DecodeShow(strings.NewReader(raw))
	require.NoError(t, err)
	require.Equal(t, []gomas.ShowKeyframe{{}, {}, {Value: 1}, {Timestamp: 2}}, show.Tracks["A"].Keyframes)
}

func TestShowKeyframeOrdering(t *testing.T) {
	for _, tc := range []struct {
		name       string
		timestamps []float64
		invalid    bool
	}{
		{"empty", nil, false}, {"zero", []float64{0}, false},
		{"ascending", []float64{0, 1, 2}, false}, {"equal", []float64{0, 1, 1}, false},
		{"descending", []float64{2, 1}, true}, {"middle inversion", []float64{0, 2, 1, 3}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			track := gomas.ShowTrack{}
			for _, ts := range tc.timestamps {
				track.Keyframes = append(track.Keyframes, gomas.ShowKeyframe{Timestamp: ts})
			}
			show := gomas.ShowData{Version: "3", Tracks: map[string]gomas.ShowTrack{"A": track}}
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
