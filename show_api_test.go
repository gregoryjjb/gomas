package main_test

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json/v2"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"
	gomas "gregoryjjb/gomas"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type apiResponse struct {
	Code int
	Body *bytes.Buffer
}

func testShowAPI(t *testing.T, storage *gomas.Storage, baseURL string) {
	show := fixtureShow(t)
	require.NoError(t, storage.CreateShow("test", show, strings.NewReader("audio bytes")))
	request := func(method, url string, body io.Reader, contentType string) *apiResponse {
		req, err := http.NewRequest(method, baseURL+url, body)
		require.NoError(t, err)
		if contentType != "" {
			req.Header.Set("Content-Type", contentType)
		}
		res, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer res.Body.Close()
		data, err := io.ReadAll(res.Body)
		require.NoError(t, err)
		return &apiResponse{Code: res.StatusCode, Body: bytes.NewBuffer(data)}
	}
	for _, route := range []string{"/api/shows/test", "/api/shows/test/data"} {
		res := request(http.MethodGet, route, nil, "")
		require.Equal(t, http.StatusOK, res.Code, res.Body.String())
		got, err := gomas.DecodeShow(res.Body)
		require.NoError(t, err)
		require.Equal(t, show, got)
	}
	raw, err := json.Marshal(show)
	require.NoError(t, err)
	res := request(http.MethodPut, "/api/shows/test/data", bytes.NewReader(raw), "application/json")
	require.Equal(t, http.StatusOK, res.Code, res.Body.String())
	for _, invalid := range []string{`{"version":"3","tracks":{}}`, `{"tracks":[]}`, `{"version":"2","tracks":[]}`, `{"version":"3","tracks":{"A":{}}}`} {
		res = request(http.MethodPut, "/api/shows/test/data", strings.NewReader(invalid), "application/json")
		require.Equal(t, http.StatusBadRequest, res.Code, res.Body.String())
		unchanged, err := storage.ReadShowData("test")
		require.NoError(t, err)
		require.Equal(t, show, unchanged)
	}
	// Export and re-import preserve the track map, editor metadata, and audio.
	res = request(http.MethodGet, "/api/export/test", nil, "")
	require.Equal(t, http.StatusOK, res.Code, res.Body.String())
	archive := append([]byte(nil), res.Body.Bytes()...)
	z, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	require.NoError(t, err)
	require.Len(t, z.File, 2)
	upload := func(filename string, contents []byte) *apiResponse {
		var body bytes.Buffer
		form := multipart.NewWriter(&body)
		file, err := form.CreateFormFile("file", filename)
		require.NoError(t, err)
		_, err = file.Write(contents)
		require.NoError(t, err)
		require.NoError(t, form.Close())
		return request(http.MethodPost, "/api/upload", &body, form.FormDataContentType())
	}
	res = upload("copy.shmr", archive)
	require.Equal(t, http.StatusOK, res.Code, res.Body.String())
	got, err := storage.ReadShowData("copy")
	require.NoError(t, err)
	require.Equal(t, show, got)
	audio, err := storage.ReadAudio("copy")
	require.NoError(t, err)
	defer audio.Close()
	contents, err := io.ReadAll(audio)
	require.NoError(t, err)
	require.Equal(t, "audio bytes", string(contents))
	var bad bytes.Buffer
	zw := zip.NewWriter(&bad)
	df, err := zw.Create("data.json")
	require.NoError(t, err)
	_, err = io.WriteString(df, `{"tracks":[]}`)
	require.NoError(t, err)
	_, err = zw.Create("audio.mp3")
	require.NoError(t, err)
	require.NoError(t, zw.Close())
	res = upload("legacy.shmr", bad.Bytes())
	require.Equal(t, http.StatusBadRequest, res.Code, res.Body.String())
	require.ErrorIs(t, storage.ShowExists("legacy"), gomas.ErrNotExist)
	// New shows serialize empty keyframes as arrays and use configured channel names.
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	require.NoError(t, form.WriteField("name", "new"))
	file, err := form.CreateFormFile("audio_file", "test.mp3")
	require.NoError(t, err)
	_, err = io.WriteString(file, "audio bytes")
	require.NoError(t, err)
	require.NoError(t, form.Close())
	res = request(http.MethodPost, "/api/shows", &body, form.FormDataContentType())
	require.Equal(t, http.StatusOK, res.Code, res.Body.String())
	got, err = storage.ReadShowData("new")
	require.NoError(t, err)
	require.Equal(t, "3", got.Version)
	require.Contains(t, got.Tracks, "Roof 1")
	require.NotNil(t, got.Tracks["Roof 1"].Keyframes)
}

func TestStorageWritesV3OnDisk(t *testing.T) {
	// Verify v3 saves and invalid-format rejection on the host filesystem.
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "songs"), 0755))
	cfgFS := gomas.NewGomasMemFS()
	require.NoError(t, cfgFS.MkdirAll(dir, 0755))
	toml := "data_dir = '" + dir + "'"
	require.NoError(t, afero.WriteFile(cfgFS, "/test.toml", []byte(toml), 0600))
	config, err := gomas.NewConfig(cfgFS, gomas.Flags{Config: "/test.toml"}, func(string) string { return "" })
	require.NoError(t, err)
	storage := gomas.NewStorage(&osTestFS{Fs: afero.NewOsFs()}, config)
	show := fixtureShow(t)
	require.NoError(t, storage.CreateShow("test", show, strings.NewReader("audio")))
	require.NoError(t, storage.WriteShowData("test", show))
	show.Version = "1"
	require.ErrorIs(t, storage.WriteShowData("test", show), gomas.ErrValidation)
	unordered := gomas.NewShowData([]string{"A"})
	unordered.Tracks["A"] = gomas.ShowTrack{Keyframes: []gomas.ShowKeyframe{{Timestamp: 2}, {Timestamp: 1}}}
	require.ErrorIs(t, storage.WriteShowData("test", unordered), gomas.ErrValidation)
	require.ErrorIs(t, storage.CreateShow("unordered", unordered, strings.NewReader("audio")), gomas.ErrValidation)
	require.ErrorIs(t, storage.ShowExists("unordered"), gomas.ErrNotExist)
	got, err := storage.ReadShowData("test")
	require.NoError(t, err)
	require.Equal(t, fixtureShow(t), got)
	files, err := os.ReadDir(filepath.Join(dir, "songs", "test"))
	require.NoError(t, err)
	require.Len(t, files, 2)
}

type osTestFS struct{ afero.Fs }

func (*osTestFS) Abs(p string) (string, error) { return filepath.Abs(p) }
func (*osTestFS) HomeDir() (string, error)     { return os.UserHomeDir() }

func TestShowDataRaw(t *testing.T) {
	config := newTestConfig(t, gomas.Flags{}, nil, `data_dir = "/data"`)
	fs := gomas.NewGomasMemFS()
	require.NoError(t, fs.MkdirAll("/data/songs/test", 0755))
	// Preserve formatting and fields the parsed representation would discard.
	raw := []byte("{\r\n  \"tracks\": {}, \"editorExtra\": true, \"version\": \"3\"\r\n}\n")
	require.NoError(t, afero.WriteFile(fs, "/data/songs/test/data.json", raw, 0644))
	storage := gomas.NewStorage(fs, config)
	handler, err := gomas.NewServerHandler(config, gomas.BuildInfo{}, nil, storage, &gomas.SlaveRegistry{})
	require.NoError(t, err)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/shows/test/data?raw=true", nil))
	require.Equal(t, http.StatusOK, response.Code)
	require.Equal(t, "application/json", response.Header().Get("Content-Type"))
	require.Equal(t, raw, response.Body.Bytes())
	digest := sha256.Sum256(response.Body.Bytes())
	hash, err := storage.ShowHash("test")
	require.NoError(t, err)
	require.Equal(t, hash, hex.EncodeToString(digest[:]))

	for _, query := range []string{"", "?raw=false"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/shows/test/data"+query, nil))
		require.Equal(t, http.StatusOK, response.Code)
		require.JSONEq(t, `{"version":"3","tracks":{}}`, response.Body.String())
	}

	for _, path := range []string{"/api/shows/missing/data?raw=true", "/api/shows/test/data?raw=true"} {
		require.NoError(t, fs.RemoveAll("/data/songs/test/data.json"))
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		require.Equal(t, http.StatusNotFound, response.Code)
	}
}
