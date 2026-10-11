package main

import (
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"
)

func TestWriteRawShowData(t *testing.T) {
	for _, backend := range []string{"memory", "disk"} {
		t.Run(backend, func(t *testing.T) {
			var fs afero.Fs = afero.NewMemMapFs()
			if backend == "disk" {
				fs = afero.NewBasePathFs(afero.NewOsFs(), t.TempDir())
			}
			storage := &Storage{fs: fs}
			dir := filepath.Join(ShowsDir, "test")
			path := filepath.Join(dir, DataFileName)
			raw := "{\r\n  \"version\": \"2\", \"tracks\": [], \"editorExtra\": true\r\n}\n"
			require.NoError(t, storage.WriteRawShowData("test", strings.NewReader(raw)))
			stored, err := afero.ReadFile(fs, path)
			require.NoError(t, err)
			require.Equal(t, raw, string(stored), "formatting and unknown fields must survive")

			require.NoError(t, storage.WriteAudio("test", strings.NewReader("audio")))
			shorter := `{"version":"2","tracks":[]}`
			require.NoError(t, storage.WriteRawShowData("test", strings.NewReader(shorter)))
			stored, err = afero.ReadFile(fs, path)
			require.NoError(t, err)
			require.Equal(t, shorter, string(stored), "replacement must not leave trailing bytes")

			downloadErr := errors.New("download interrupted")
			reader := io.MultiReader(strings.NewReader("partial data"), rawShowErrorReader{downloadErr})
			require.ErrorIs(t, storage.WriteRawShowData("test", reader), downloadErr)
			audio, err := afero.ReadFile(fs, filepath.Join(dir, AudioFileName))
			require.NoError(t, err)
			require.Equal(t, "audio", string(audio))
			entries, err := afero.ReadDir(fs, dir)
			require.NoError(t, err)
			require.Len(t, entries, 2)
			require.ErrorIs(t, storage.WriteRawShowData("bad/name", strings.NewReader(raw)), ErrValidation)
		})
	}
}

type rawShowErrorReader struct{ err error }

func (r rawShowErrorReader) Read([]byte) (int, error) { return 0, r.err }
