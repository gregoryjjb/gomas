package main

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime/multipart"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/spf13/afero"
)

var (
	ErrNotExist = errors.New("doesn't exist")
	ErrExists   = errors.New("already exists")
)

// ShowsDir is the subdirectory under the data dir where the shows go
const ShowsDir = "songs"

const (
	DataFileName  = "data.json"
	AudioFileName = "audio.mp3"
)

var badNameRegex = regexp.MustCompile(`[<>:"/\\|?\*]`)

func ValidateShowName(name string) error {
	if name == "" {
		return fmt.Errorf("%w: show name cannot be blank", ErrValidation)
	}

	m := badNameRegex.FindAllString(name, -1)

	if len(m) > 0 {
		return fmt.Errorf("%w: show name contains disallowed characters %s", ErrValidation, strings.Join(m, " "))
	}

	return nil
}

type Storage struct {
	config *Config
	fs     afero.Fs
}

func NewStorage(fs GomasFS, config *Config) *Storage {
	subFS := afero.NewBasePathFs(fs, config.DataDir())

	return &Storage{
		config: config,
		fs:     subFS,
	}
}

func (s *Storage) Migrate() error {
	projectEntries, err := afero.ReadDir(s.fs, "projects")
	if err != nil {
		return err
	}
	var projectFiles []string
	for _, entry := range projectEntries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".json" {
			projectFiles = append(projectFiles, entry.Name())
		}
	}

	audioEntries, err := afero.ReadDir(s.fs, "audio")
	if err != nil {
		return err
	}
	var audioFiles []string
	for _, entry := range audioEntries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".mp3" {
			audioFiles = append(audioFiles, entry.Name())
		}
	}

	for _, projectFile := range projectFiles {
		name := strings.TrimSuffix(projectFile, filepath.Ext(projectFile))

		from := filepath.Join("projects", projectFile)
		to := filepath.Join(ShowsDir, name, DataFileName)

		fmt.Printf("Migrating %q to %q\n", from, to)

		f, err := s.fs.Open(from)
		if err != nil {
			return err
		}
		defer f.Close()
		
		var legacy LegacyShow
		if err := json.NewDecoder(f).Decode(&legacy); err != nil {
			return err
		}

		var modern ProjectData
		for _, track := range legacy.Tracks {
			var newTrack Track

			for _, keyframe := range track.Keyframes {
				newTrack.Keyframes = append(newTrack.Keyframes, Keyframe{
					Timestamp: keyframe.Time,
					Value: float64(keyframe.State),
					Selected: keyframe.Selected,
				})
			}

			modern.Tracks = append(modern.Tracks, newTrack)
		}

		if err := s.fs.MkdirAll(filepath.Dir(to), 0777); err != nil {
			return err
		}
		toFile, err := s.fs.Create(to)
		if err != nil {
			return err
		}
		if err := json.NewEncoder(toFile).Encode(modern); err != nil {
			return err
		}
	}

	for _, audioFile := range audioFiles {
		name := strings.TrimSuffix(audioFile, filepath.Ext(audioFile))

		from := filepath.Join("audio", audioFile)
		to := filepath.Join(ShowsDir, name, AudioFileName)

		fmt.Printf("Migrating %q to %q\n", from, to)

		if err := s.fs.MkdirAll(filepath.Dir(to), 0777); err != nil {
			return err
		}
		if err := s.fs.Rename(from, to); err != nil {
			return err
		}
	}

	return nil
}

//////////////
// Playlists

type PlaylistShow struct {
	ID     string `json:"id"`
	Exists bool
}

type Playlist struct {
	ID    string   `json:"id"`
	Shows []string `json:"shows"`
}

func (s *Storage) ListPlaylists() ([]Playlist, error) {
	file, err := s.fs.Open("playlists.json")
	if err != nil {
		if errors.Is(err, afero.ErrFileNotFound) {
			return nil, nil
		}
		return nil, err
	}
	defer file.Close()

	// shows, err := ListShows()
	// if err != nil {
	// 	return nil, err
	// }
	// showset := make(map[string]ShowInfo)
	// for _, show := range shows {
	// 	showset[show.ID] = *show
	// }

	var data []Playlist
	if err := json.NewDecoder(file).Decode(&data); err != nil {
		return nil, err
	}

	// for i, playlist := range data {
	// 	for j, show := range playlist.Shows {
	// 		if _, exists := showset[show.ID]; !exists {
	// 			show.Exists = false
	// 			data[i].Shows[j] = show
	// 		}
	// 	}
	// }

	return data, nil
}

//////////
// Shows

func (s *Storage) ListShows() ([]string, error) {
	entries, err := afero.ReadDir(s.fs, ShowsDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}

	var shows []string
	for _, entry := range entries {
		if entry.IsDir() {
			shows = append(shows, entry.Name())
		}
	}

	return shows, nil
}

// ShowExists returns an error if the named show does not exist
func (s *Storage) ShowExists(name string) error {
	if err := ValidateShowName(name); err != nil {
		return err
	}

	info, err := s.fs.Stat(filepath.Join(ShowsDir, name))
	if err != nil {
		if errors.Is(err, afero.ErrFileNotFound) {
			return fmt.Errorf("show %q %w", name, ErrNotExist)
		}

		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("show %q is not a directory", name)
	}

	return nil
}

// ShowNotExists returns an error if the named show already exists
func (s *Storage) ShowNotExists(name string) error {
	if err := ValidateShowName(name); err != nil {
		return err
	}

	if _, err := s.fs.Stat(filepath.Join(ShowsDir, name)); !errors.Is(err, afero.ErrFileNotFound) {
		if err == nil {
			return fmt.Errorf("%w: show %q", ErrExists, name)
		}
		return err
	}

	return nil
}

// CreateShow creates a new show
func (s *Storage) CreateShow(name string, data ProjectData, audio io.Reader) error {
	if err := s.ShowNotExists(name); err != nil {
		return err
	}

	if err := s.fs.Mkdir(filepath.Join(ShowsDir, name), 0755); err != nil {
		return err
	}

	if err := s.WriteShowData(name, data); err != nil {
		return err
	}

	if err := s.WriteAudio(name, audio); err != nil {
		return err
	}

	return nil
}

func (s *Storage) WriteShowData(name string, data ProjectData) error {
	if err := s.ShowExists(name); err != nil {
		return err
	}

	f, err := s.fs.Create(filepath.Join(ShowsDir, name, DataFileName))
	if err != nil {
		return err
	}
	defer f.Close()

	if err := json.NewEncoder(f).Encode(data); err != nil {
		return err
	}

	return f.Sync()
}

func (s *Storage) WriteAudio(name string, audio io.Reader) error {
	if err := s.ShowExists(name); err != nil {
		return err
	}

	f, err := s.fs.Create(filepath.Join(ShowsDir, name, AudioFileName))
	if err != nil {
		return err
	}
	defer f.Close()

	if _, err := io.Copy(f, audio); err != nil {
		return err
	}

	return f.Sync()
}

func (s *Storage) ReadShowData(name string) (ProjectData, error) {
	if err := s.ShowExists(name); err != nil {
		return ProjectData{}, err
	}

	f, err := s.fs.Open(filepath.Join(ShowsDir, name, DataFileName))
	if err != nil {
		return ProjectData{}, err
	}
	defer f.Close()

	var data ProjectData
	err = json.NewDecoder(f).Decode(&data)
	return data, err
}

func (s *Storage) ReadAudio(name string) (afero.File, error) {
	if err := s.ShowExists(name); err != nil {
		return nil, err
	}

	return s.fs.Open(filepath.Join(ShowsDir, name, AudioFileName))
}

// TODO: create a better interface between the upload
func (s *Storage) ImportShmr(file multipart.File, header *multipart.FileHeader) error {
	z, err := zip.NewReader(file, header.Size)
	if err != nil {
		return err
	}

	dataFile, err := z.Open(DataFileName)
	if err != nil {
		return err
	}
	defer dataFile.Close()

	audioFile, err := z.Open(AudioFileName)
	if err != nil {
		return err
	}

	var data ProjectData
	if err := json.NewDecoder(dataFile).Decode(&data); err != nil {
		return err
	}

	if data.Tracks == nil {
		return fmt.Errorf("no tracks present")
	}

	name := strings.TrimSuffix(header.Filename, filepath.Ext(header.Filename))

	return s.CreateShow(name, data, audioFile)
}

// ExportShmr bundles the specified show and writes it to w
func (s *Storage) ExportShmr(name string, w io.Writer) error {
	z := zip.NewWriter(w)

	showFS := afero.NewIOFS(afero.NewBasePathFs(s.fs, filepath.Join(ShowsDir, name)))
	if err := z.AddFS(showFS); err != nil {
		return err
	}

	return z.Close()
}

func (s *Storage) RenameShow(from, to string) error {
	if err := s.ShowExists(from); err != nil {
		return err
	}
	if err := s.ShowNotExists(to); err != nil {
		return err
	}

	fromPath := filepath.Join(ShowsDir, from)
	toPath := filepath.Join(ShowsDir, to)

	return s.fs.Rename(fromPath, toPath)
}
