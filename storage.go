package main

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	ErrNotExist = errors.New("doesn't exist")
	ErrExists   = errors.New("already exists")
)

func ShowDir() string {
	return filepath.Join(GetDataDir(), "projects")
}

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

////////////
// Helpers

// Exists returns true if the path points to an existing file OR folder
func Exists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}

	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}

	return false, err
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

func ListPlaylists() ([]Playlist, error) {
	path := filepath.Join(GetDataDir(), "playlists.json")
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
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

func ListShows() ([]string, error) {
	entries, err := os.ReadDir(ShowDir())
	if err != nil {
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
func ShowExists(name string) error {
	if err := ValidateShowName(name); err != nil {
		return err
	}

	info, err := os.Stat(filepath.Join(ShowDir(), name))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
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
func ShowNotExists(name string) error {
	if err := ValidateShowName(name); err != nil {
		return err
	}

	if _, err := os.Stat(filepath.Join(ShowDir(), name)); !errors.Is(err, os.ErrNotExist) {
		if err == nil {
			return fmt.Errorf("%w: show %q", ErrExists, name)
		}
		return err
	}

	return nil
}

func ShowAudioPath(name string) (string, error) {
	audioPath := filepath.Join(ShowDir(), name, "audio.mp3")
	if _, err := os.Stat(audioPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("audio for show %q %w", name, ErrNotExist)
		}
		return "", err
	}

	return audioPath, nil
}

// CreateShow creates a new show
func CreateShow(name string, data ProjectData, audio io.Reader) error {
	if err := ShowNotExists(name); err != nil {
		return err
	}

	if err := os.Mkdir(filepath.Join(ShowDir(), name), 0755); err != nil {
		return err
	}

	if err := WriteShowData(name, data); err != nil {
		return err
	}

	if err := WriteAudio(name, audio); err != nil {
		return err
	}

	return nil
}

func WriteShowData(name string, data ProjectData) error {
	if err := ShowExists(name); err != nil {
		return err
	}

	f, err := os.Create(filepath.Join(ShowDir(), name, "data.json"))
	if err != nil {
		return err
	}
	defer f.Close()

	if err := json.NewEncoder(f).Encode(data); err != nil {
		return err
	}

	return f.Sync()
}

func WriteAudio(name string, audio io.Reader) error {
	if err := ShowExists(name); err != nil {
		return err
	}

	f, err := os.Create(filepath.Join(ShowDir(), name, "audio.mp3"))
	if err != nil {
		return err
	}
	defer f.Close()

	if _, err := io.Copy(f, audio); err != nil {
		return err
	}

	return f.Sync()
}

func ReadShowData(name string) (ProjectData, error) {
	if err := ShowExists(name); err != nil {
		return ProjectData{}, err
	}

	f, err := os.Open(filepath.Join(ShowDir(), name, "data.json"))
	if err != nil {
		return ProjectData{}, err
	}
	defer f.Close()

	var data ProjectData
	err = json.NewDecoder(f).Decode(&data)
	return data, err
}

func ReadAudio(name string) (*os.File, error) {
	if err := ShowExists(name); err != nil {
		return nil, err
	}

	return os.Open(filepath.Join(ShowDir(), name, "audio.mp3"))
}

// TODO: create a better interface between the upload
func ImportShmr(file multipart.File, header *multipart.FileHeader) error {
	z, err := zip.NewReader(file, header.Size)
	if err != nil {
		return err
	}

	dataFile, err := z.Open("data.json")
	if err != nil {
		return err
	}
	defer dataFile.Close()

	audioFile, err := z.Open("audio.mp3")
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

	return CreateShow(name, data, audioFile)
}

// ExportShmr bundles the specified show and writes it to w
func ExportShmr(name string, w io.Writer) error {
	z := zip.NewWriter(w)

	showFS := os.DirFS(filepath.Join(ShowDir(), name))
	if err := z.AddFS(showFS); err != nil {
		return err
	}

	return z.Close()
}

func RenameShow(from, to string) error {
	if err := ShowExists(from); err != nil {
		return err
	}
	if err := ShowNotExists(to); err != nil {
		return err
	}

	fromPath := filepath.Join(ShowDir(), from)
	toPath := filepath.Join(ShowDir(), to)

	return os.Rename(fromPath, toPath)
}
