package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"
)

func TestPlaySlaveEndpoint(t *testing.T) {
	const name = "A & B + snow"
	const raw = `{"version":"2","tracks":[]}`
	digest := sha256.Sum256([]byte(raw))
	hash := hex.EncodeToString(digest[:])
	startedAt := time.UnixMicro(123456789)
	for _, tc := range []struct {
		name   string
		param  string
		value  string
		role   Role
		remote string
		status int
		body   string
	}{
		{name: "matching", status: http.StatusNoContent},
		{name: "uppercase digest", param: "sha256", value: strings.ToUpper(hash), status: http.StatusNoContent},
		{name: "missing name", param: "name", status: http.StatusBadRequest},
		{name: "invalid name", param: "name", value: "../show", status: http.StatusBadRequest},
		{name: "missing hash", param: "sha256", status: http.StatusBadRequest},
		{name: "short hash", param: "sha256", value: "ab", status: http.StatusBadRequest},
		{name: "long hash", param: "sha256", value: strings.Repeat("a", 66), status: http.StatusBadRequest},
		{name: "nonhex hash", param: "sha256", value: strings.Repeat("z", 64), status: http.StatusBadRequest},
		{name: "missing timestamp", param: "slaveStartTimeMicro", status: http.StatusBadRequest},
		{name: "invalid timestamp", param: "slaveStartTimeMicro", value: "tomorrow", status: http.StatusBadRequest},
		{name: "overflow timestamp", param: "slaveStartTimeMicro", value: "9223372036854775808", status: http.StatusBadRequest},
		{name: "zero Go timestamp", param: "slaveStartTimeMicro", value: MarshalStartTime(time.Time{}), status: http.StatusBadRequest},
		{name: "missing port", param: "masterPort", status: http.StatusBadRequest},
		{name: "zero port", param: "masterPort", value: "0", status: http.StatusBadRequest},
		{name: "negative port", param: "masterPort", value: "-1", status: http.StatusBadRequest},
		{name: "large port", param: "masterPort", value: "65536", status: http.StatusBadRequest},
		{name: "nonnumeric port", param: "masterPort", value: "http", status: http.StatusBadRequest},
		{name: "master role", role: RoleMaster, status: http.StatusBadRequest},
		{name: "standalone role", role: RoleStandalone, status: http.StatusBadRequest},
		{name: "invalid remote address", remote: "not-an-address", status: http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			role := tc.role
			if role == "" {
				role = RoleSlave
			}
			config := &Config{}
			config.toml.Store(&ConfigTOML{Role: role})
			storage := &Storage{fs: afero.NewMemMapFs()}
			require.NoError(t, storage.fs.MkdirAll(filepath.Join(ShowsDir, name), 0755))
			require.NoError(t, afero.WriteFile(storage.fs, filepath.Join(ShowsDir, name, DataFileName), []byte(raw), 0644))
			player := &Player{commandChannel: make(chan PlayerMessage, 1)}
			handler, err := NewServerHandler(config, BuildInfo{}, player, storage)
			require.NoError(t, err)
			query := url.Values{
				"name": {name}, "sha256": {hash}, "masterPort": {"1225"},
				"slaveStartTimeMicro": {MarshalStartTime(startedAt)},
			}
			if tc.param != "" {
				query.Set(tc.param, tc.value)
			}
			request := httptest.NewRequest(http.MethodGet, "/api/playSlave?"+query.Encode(), nil)
			request.RemoteAddr = "192.0.2.10:50123"
			if tc.remote != "" {
				request.RemoteAddr = tc.remote
			}
			request.Header.Set("X-Forwarded-For", "198.51.100.99")
			request.Host = "slave.example:1226"
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			require.Equal(t, tc.status, response.Code, response.Body.String())
			if tc.body != "" {
				require.Contains(t, response.Body.String(), tc.body)
			}
			if tc.status == http.StatusNoContent {
				require.Len(t, player.commandChannel, 1)
				require.Equal(t, commandPlay{id: name, startedAt: startedAt}, <-player.commandChannel)
			} else {
				require.Empty(t, player.commandChannel, "rejected requests must not start playback")
			}
		})
	}
}

func TestPlaySlaveDownload(t *testing.T) {
	const name = "A & B + snow#one"
	const raw = "{\r\n  \"version\": \"2\", \"tracks\": [], \"editorExtra\": true\r\n}\n"
	const old = `{"version":"2","tracks":[]}`
	startedAt := time.UnixMicro(123456789)
	for _, tc := range []struct {
		name          string
		local         string
		payload       string
		status        int
		wrongHash     bool
		truncated     bool
		readOnly      bool
		cancelled     bool
		wantPlay      bool
		wantDownloads int32
	}{
		{name: "missing show", wantPlay: true, wantDownloads: 1},
		{name: "stale show", local: old, wantPlay: true, wantDownloads: 1},
		{name: "matching show skips download", local: raw, wantPlay: true},
		{name: "wrong hash", local: old, wrongHash: true, wantDownloads: 1},
		{name: "invalid JSON", local: old, payload: "{", wantDownloads: 1},
		{name: "invalid format", local: old, payload: `{"version":"1","tracks":[]}`, wantDownloads: 1},
		{name: "master HTTP error", local: old, status: http.StatusNotFound, wantDownloads: 1},
		{name: "incomplete download", local: old, truncated: true, wantDownloads: 1},
		{name: "save failure", local: old, readOnly: true, wantDownloads: 1},
		{name: "cancelled request", cancelled: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			payload := tc.payload
			if payload == "" {
				payload = raw
			}
			digest := sha256.Sum256([]byte(payload))
			hash := hex.EncodeToString(digest[:])
			if tc.wrongHash {
				hash = strings.Repeat("0", 64)
			}
			var downloads atomic.Int32
			master := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				downloads.Add(1)
				if r.Method != http.MethodGet || r.URL.Path != "/api/shows/"+name+"/data" || r.URL.Query().Get("raw") != "true" {
					t.Errorf("unexpected download request: %s %s", r.Method, r.URL)
					http.Error(w, "unexpected download request", http.StatusBadRequest)
					return
				}
				if tc.status != 0 {
					w.WriteHeader(tc.status)
					return
				}
				if tc.truncated {
					w.Header().Set("Content-Length", strconv.Itoa(len(payload)+10))
				}
				_, _ = w.Write([]byte(payload))
			}))
			defer master.Close()
			address := master.Listener.Addr().(*net.TCPAddr)
			config := &Config{}
			config.toml.Store(&ConfigTOML{Role: RoleSlave})
			fs := afero.NewMemMapFs()
			path := filepath.Join(ShowsDir, name, DataFileName)
			if tc.local != "" {
				require.NoError(t, fs.MkdirAll(filepath.Dir(path), 0755))
				require.NoError(t, afero.WriteFile(fs, path, []byte(tc.local), 0644))
			}
			storage := &Storage{fs: fs}
			if tc.readOnly {
				storage.fs = afero.NewReadOnlyFs(fs)
			}
			player := &Player{commandChannel: make(chan PlayerMessage, 1)}
			handler, err := NewServerHandler(config, BuildInfo{}, player, storage)
			require.NoError(t, err)
			query := url.Values{
				"name": {name}, "sha256": {hash}, "masterPort": {strconv.Itoa(address.Port)},
				"slaveStartTimeMicro": {MarshalStartTime(startedAt)},
			}
			request := httptest.NewRequest(http.MethodGet, "/api/playSlave?"+query.Encode(), nil)
			// The callback must use the advertised listener port, not this source port.
			request.RemoteAddr = net.JoinHostPort(address.IP.String(), "50123")
			request.Header.Set("X-Forwarded-For", "192.0.2.99")
			if tc.cancelled {
				ctx, cancel := context.WithCancel(request.Context())
				cancel()
				request = request.WithContext(ctx)
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			require.Equal(t, tc.wantDownloads, downloads.Load())
			stored, readErr := afero.ReadFile(fs, path)
			if tc.wantPlay {
				require.Equal(t, http.StatusNoContent, response.Code, response.Body.String())
				require.NoError(t, readErr)
				require.Equal(t, payload, string(stored), "saved bytes must match the master exactly")
				require.Len(t, player.commandChannel, 1)
				require.Equal(t, commandPlay{id: name, startedAt: startedAt}, <-player.commandChannel)
			} else {
				require.GreaterOrEqual(t, response.Code, 400, response.Body.String())
				require.Empty(t, player.commandChannel, "failed synchronization must not start playback")
				if tc.local != "" {
					require.NoError(t, readErr)
					require.Equal(t, tc.local, string(stored))
				} else {
					require.ErrorIs(t, readErr, afero.ErrFileNotFound)
				}
			}
		})
	}
}
