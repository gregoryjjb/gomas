package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
)

type LegacyShow struct {
	ProjectData *LegacyProjectData `json:"projectData"`
	Tracks      []*LegacyTrack     `json:"tracks"`
}

type LegacyProjectData struct {
	Name string `json:"name"`
	ID   string `json:"id"`
}

type LegacyTrack struct {
	ID        int               `json:"id"`
	Keyframes []*LegacyKeyframe `json:"keyframes"`
}

type LegacyKeyframe struct {
	Channel  int         `json:"channel"`
	Time     float64     `json:"time"`
	OldTime  float64     `json:"oldTime"`
	State    LegacyState `json:"state"`
	Selected bool        `json:"selected"`
}

type LegacyState int

func (state *LegacyState) UnmarshalJSON(data []byte) error {
	asString := string(data)
	if asString == "1" || asString == "true" {
		*state = 1
	} else if asString == "0" || asString == "false" {
		*state = 0
	} else {
		return fmt.Errorf("state unmarshal error: invalid input %s", asString)
	}
	return nil
}

func NewShow(id string) *LegacyShow {
	show := &LegacyShow{
		ProjectData: &LegacyProjectData{
			ID:   id,
			Name: id,
		},
	}

	// Hack: hardcoded 8 channels
	for i := 0; i < 8; i++ {
		show.Tracks = append(show.Tracks, &LegacyTrack{
			ID:        i,
			Keyframes: []*LegacyKeyframe{},
		})
	}

	return show
}

var ErrNoAudioFile = errors.New("audio file does not exist")

func ShowDir() string {
	result, _ := filepath.Abs(filepath.Join(DataDir, "projects"))
	return result
}

func AudioDir() string {
	result, _ := filepath.Abs(filepath.Join(DataDir, "audio"))
	return result
}

func ValidateShowID(id string) error {
	m, _ := regexp.MatchString(`^[0-9A-Za-z_\- ]+$`, id)
	if !m {
		return fmt.Errorf("Show name must contain only letters, numbers, dashes, underscores, and spaces")
	}
	return nil
}

////////////
// Helpers

func JoinExtension(base string, ext string) string {
	return fmt.Sprintf("%s.%s", base, ext)
}

// Removes the extension from the given path or filename
func TrimExtension(path string) string {
	filename := filepath.Base(path)
	extension := filepath.Ext(filename)

	return filename[0 : len(filename)-len(extension)]
}

func FileExists(path string) (bool, error) {
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
	path := filepath.Join(DataDir, "playlists.json")
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	defer file.Close()

	shows, err := ListShows()
	if err != nil {
		return nil, err
	}
	showset := make(map[string]ShowInfo)
	for _, show := range shows {
		showset[show.ID] = *show
	}

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

type ShowInfo struct {
	ID       string `json:"id"`
	HasAudio bool   `json:"hasAudio"`
}

func ListShows() ([]*ShowInfo, error) {
	entries, err := os.ReadDir(ShowDir())
	if err != nil {
		return nil, err
	}

	audioEntries, err := os.ReadDir(AudioDir())
	if err != nil {
		return nil, err
	}

	audioSet := make(map[string]bool)
	for _, audioEntry := range audioEntries {
		audioSet[audioEntry.Name()] = true
	}

	var results []*ShowInfo

	for _, entry := range entries {
		filename := entry.Name()
		extension := filepath.Ext(filename)

		if extension == ".json" {
			id := TrimExtension(filename)

			expectedAudioName := JoinExtension(id, "mp3")
			hasAudio := audioSet[expectedAudioName]

			results = append(results, &ShowInfo{
				ID:       id,
				HasAudio: hasAudio,
			})
		}
	}

	return results, nil
}

func showIDToPath(id string) string {
	return filepath.Join(DataDir, "projects", JoinExtension(id, "json"))
}

func ShowExists(id string) bool {
	showPath := showIDToPath(id)
	exists, _ := FileExists(showPath)
	return exists
}

func LoadShow(id string) (*LegacyShow, error) {
	showPath := showIDToPath(id)

	file, err := os.Open(showPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var data LegacyShow

	if err := json.NewDecoder(file).Decode(&data); err != nil {
		return nil, err
	}

	return &data, nil
}

func SaveShow(id string, show *LegacyShow) error {
	showPath := showIDToPath(id)

	file, err := os.OpenFile(showPath, os.O_CREATE, os.ModePerm)
	if err != nil {
		return err
	}
	defer file.Close()
	return json.NewEncoder(file).Encode(show)
}

func ShowAudioPath(id string) (string, error) {
	audioPath, err := filepath.Abs(filepath.Join(DataDir, "audio", JoinExtension(id, "mp3")))
	if err != nil {
		return "", err
	}
	exists, err := FileExists(audioPath)
	if err != nil {
		return "", err
	}
	if !exists {
		return "", ErrNoAudioFile // fmt.Errorf("audio file does not exist: %s", audioPath)
	}

	return audioPath, nil
}

func SaveShowAudio(id string, newAudio io.Reader) error {
	audioPath := filepath.Join(DataDir, "audio", JoinExtension(id, "mp3"))
	file, err := os.OpenFile(audioPath, os.O_CREATE, os.ModePerm)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = io.Copy(file, newAudio)
	return err
}

func ShowIsPlayable(id string) (bool, error) {
	if !ShowExists(id) {
		return false, nil
	}

	audioPath, err := ShowAudioPath(id)
	if err == ErrNoAudioFile {
		return false, nil
	} else if err != nil {
		return false, err
	}

	return audioPath != "", nil
}

type FlatKeyframe struct {
	Time   float64
	States []bool
}

// Sortable keyframes
type StandaloneKeyframe struct {
	Time       float64
	State      bool
	TrackIndex int
}
type StandaloneKeyframes []*StandaloneKeyframe

func (kfs StandaloneKeyframes) Len() int {
	return len(kfs)
}
func (kfs StandaloneKeyframes) Swap(i, j int) {
	kfs[i], kfs[j] = kfs[j], kfs[i]
}
func (kfs StandaloneKeyframes) Less(i, j int) bool {
	return kfs[i].Time < kfs[j].Time
}

func CloseTogether(a float64, b float64) bool {
	return math.Abs(a-b) < 0.001
}

func LoadFlatKeyframes(id string) ([]*FlatKeyframe, error) {
	show, err := LoadShow(id)
	if err != nil {
		return nil, err
	}

	var totalKeyframeCount int
	for _, track := range show.Tracks {
		totalKeyframeCount += len(track.Keyframes)
	}

	sourceKeyframes := make([]*StandaloneKeyframe, 0, totalKeyframeCount)
	for trackIndex, track := range show.Tracks {
		for _, keyframe := range track.Keyframes {
			sourceKeyframes = append(sourceKeyframes, &StandaloneKeyframe{
				Time:       keyframe.Time,
				State:      keyframe.State == 1,
				TrackIndex: trackIndex,
			})
		}
	}
	sort.Sort(StandaloneKeyframes(sourceKeyframes))

	trackCount := len(show.Tracks)

	var flatKeyframes []*FlatKeyframe
	kf := &FlatKeyframe{
		Time:   0,
		States: make([]bool, trackCount),
	}

	pushCurrentKeyframe := func(newTime float64) {
		flatKeyframes = append(flatKeyframes, kf)

		newStates := make([]bool, len(kf.States))
		copy(newStates, kf.States)

		newKF := &FlatKeyframe{
			Time:   newTime,
			States: newStates,
		}
		kf = newKF
	}

	for _, sourceKeyframe := range sourceKeyframes {
		// Times are far apart, need to clone the current keyframe
		if !CloseTogether(sourceKeyframe.Time, kf.Time) {
			pushCurrentKeyframe(sourceKeyframe.Time)
		}

		kf.States[sourceKeyframe.TrackIndex] = sourceKeyframe.State
	}

	// Debug: dump keyframes to file
	// Create a file for writing
	// f, _ := os.Create("./debug.txt")
	// // Create a writer
	// w := bufio.NewWriter(f)
	// for _, kf := range flatKeyframes {
	// 	stateStrings := make([]string, 0, len(kf.States))
	// 	for _, s := range kf.States {
	// 		v := "0"
	// 		if s {
	// 			v = "1"
	// 		}
	// 		stateStrings = append(stateStrings, v)
	// 	}
	// 	line := fmt.Sprintf("%f,%s\n", kf.Time, strings.Join(stateStrings, ","))
	// 	w.WriteString(line)
	// }
	// // Very important to invoke after writing a large number of lines
	// w.Flush()
	// f.Close()

	return flatKeyframes, nil
}
