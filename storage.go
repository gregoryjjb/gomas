package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
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

var ErrNoAudioFile = errors.New("audio file does not exist")

var DataDir string
func SetDataDir() {
	provided := os.Getenv("GOMAS_DATA_DIR")
	if provided == "" {
		DataDir, _ = filepath.Abs("./data")
		fmt.Printf("using default data directory '%s'\n", DataDir)
	} else {
		DataDir, _ = filepath.Abs(provided)
		fmt.Printf("using provided data directory '%s'\n", DataDir)
	}
}

func ShowDir() string {
	result, _ := filepath.Abs(filepath.Join(DataDir, "projects"))
	return result
}

func AudioDir() string {
	result, _ := filepath.Abs(filepath.Join(DataDir, "audio"))
	return result
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

/////////
// Init

func init() {
	SetDataDir()
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

func LoadShow(id string) (*LegacyShow, error) {
	showPath, err := filepath.Abs(filepath.Join(DataDir, "projects", JoinExtension(id, "json")))
	if err != nil {
		return nil, err
	}

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

func ShowExists(id string) (bool, error) {
	showPath := filepath.Join(ShowDir(), JoinExtension(id, "json"))
	exists, err := FileExists(showPath)
	if err != nil {
		return false, err
	}
	if !exists {
		return false, nil
	}

	audioPath := filepath.Join(AudioDir(), JoinExtension(id, "mp3"))
	exists, err = FileExists(audioPath)
	if err != nil {
		return false, err
	}
	return exists, nil
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
	f, _ := os.Create("./debug.txt")
	// Create a writer
	w := bufio.NewWriter(f)
	for _, kf := range flatKeyframes {
		stateStrings := make([]string, 0, len(kf.States))
		for _, s := range kf.States {
			v := "0"
			if s {
				v = "1"
			}
			stateStrings = append(stateStrings, v)
		}
		line := fmt.Sprintf("%f,%s\n", kf.Time, strings.Join(stateStrings, ","))
		w.WriteString(line)
	}
	// Very important to invoke after writing a large number of lines
	w.Flush()
	f.Close()

	return flatKeyframes, nil
}
