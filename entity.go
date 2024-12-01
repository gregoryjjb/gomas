package main

import (
	"fmt"
	"math"
	"sort"
)

type Project struct {
	Name string
	Data ProjectData
}

type ProjectData struct {
	Tracks []Track `json:"tracks"`
}

type Track struct {
	Name      string     `json:"name"`
	Keyframes []Keyframe `json:"keyframes"`
}

type Keyframe struct {
	Timestamp float64 `json:"ts"`
	Value     float64 `json:"value"`
	Selected  bool    `json:"selected,omitempty"`
}

type FlatKeyframe struct {
	Time   float64
	States []bool
}

// Sortable keyframes
type standaloneKeyframe struct {
	Time       float64
	State      bool
	TrackIndex int
}
type standaloneKeyframes []*standaloneKeyframe

func (kfs standaloneKeyframes) Len() int {
	return len(kfs)
}
func (kfs standaloneKeyframes) Swap(i, j int) {
	kfs[i], kfs[j] = kfs[j], kfs[i]
}
func (kfs standaloneKeyframes) Less(i, j int) bool {
	return kfs[i].Time < kfs[j].Time
}

func CloseTogether(a float64, b float64) bool {
	return math.Abs(a-b) < 0.001
}

func (pd *ProjectData) FlatKeyframes() []FlatKeyframe {
	var totalKeyframeCount int
	for _, track := range pd.Tracks {
		totalKeyframeCount += len(track.Keyframes)
	}

	sourceKeyframes := make([]*standaloneKeyframe, 0, totalKeyframeCount)
	for trackIndex, track := range pd.Tracks {
		for _, keyframe := range track.Keyframes {
			sourceKeyframes = append(sourceKeyframes, &standaloneKeyframe{
				Time:       keyframe.Timestamp,
				State:      keyframe.Value == 1,
				TrackIndex: trackIndex,
			})
		}
	}
	sort.Sort(standaloneKeyframes(sourceKeyframes))

	trackCount := len(pd.Tracks)

	var flatKeyframes []FlatKeyframe
	kf := FlatKeyframe{
		Time:   0,
		States: make([]bool, trackCount),
	}

	pushCurrentKeyframe := func(newTime float64) {
		flatKeyframes = append(flatKeyframes, kf)

		newStates := make([]bool, len(kf.States))
		copy(newStates, kf.States)

		newKF := FlatKeyframe{
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

	return flatKeyframes
}

func NewProjectData(trackCount int) ProjectData {
	tracks := make([]Track, 0, trackCount)
	for i := 0; i < trackCount; i++ {
		tracks = append(tracks, Track{
			Name: fmt.Sprintf("Channel %d", i),
		})
	}

	return ProjectData{
		Tracks: tracks,
	}
}

// Legacy

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
