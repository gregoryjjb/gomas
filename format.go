package main

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
	"io"
	"math"
	"strings"
)

// ShowFormatVersion is the version string stored in data.json.
const ShowFormatVersion = "3"

// BeatTrackID is reserved for editor beat markers and never drives GPIO.
const BeatTrackID = "Beat"

// ShowData is data.json in a .shmr ZIP alongside audio.mp3. Track names are
// GPIO mapping keys; the editor's layout is stored separately from the show.
// Only version 3 is supported.
type ShowData struct {
	Version   string               `json:"version"`
	UpdatedAt string               `json:"updatedAt,omitempty"`
	Tracks    map[string]ShowTrack `json:"tracks"`
}

// ShowTrack contains lighting events and editor metadata for a named channel.
// Locked only affects editing.
type ShowTrack struct {
	Keyframes []ShowKeyframe `json:"keyframes"`
	Locked    bool           `json:"locked"`
}

// ShowKeyframe sets a value at a time in seconds from the start of the audio.
// Positive values mean on. Selected is editor metadata, not playback state.
type ShowKeyframe struct {
	Timestamp float64 `json:"ts"`
	Value     float64 `json:"value"`
	Selected  bool    `json:"selected,omitzero"`
}

func (data *ShowData) UnmarshalJSONFrom(dec *jsontext.Decoder) error {
	type plain ShowData
	var result plain
	if err := json.UnmarshalDecode(dec, &result); err != nil {
		return err
	}
	if result.Tracks == nil {
		return fmt.Errorf("tracks must be an object")
	}
	for name, track := range result.Tracks {
		if track.Keyframes == nil {
			return fmt.Errorf("track %q: keyframes must be an array", name)
		}
	}
	show := ShowData(result)
	if err := show.Validate(); err != nil {
		return err
	}
	*data = show
	return nil
}

// DecodeShow is the common entry point for disk, archive, and API reads.
func DecodeShow(r io.Reader) (ShowData, error) {
	var data ShowData
	if err := json.UnmarshalRead(r, &data); err != nil {
		return ShowData{}, fmt.Errorf("%w: invalid show: %w", ErrValidation, err)
	}
	return data, nil
}

// Validate checks track names and keyframes before saving or playing.
// Nil Go maps and slices are valid: JSON v2 writes them as {} and [].
func (data ShowData) Validate() error {
	if data.Version != ShowFormatVersion {
		return fmt.Errorf("unsupported show version %q (expected %q)", data.Version, ShowFormatVersion)
	}
	for name, track := range data.Tracks {
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("track name must not be empty")
		}
		for i, kf := range track.Keyframes {
			if math.IsNaN(kf.Timestamp) || math.IsInf(kf.Timestamp, 0) || kf.Timestamp < 0 || math.IsNaN(kf.Value) || math.IsInf(kf.Value, 0) {
				return fmt.Errorf("track %q keyframe %d: expected nonnegative finite ts and finite value", name, i)
			}
			if i > 0 && kf.Timestamp < track.Keyframes[i-1].Timestamp {
				return fmt.Errorf("track %q keyframe %d: keyframes must be in ascending order by timestamp", name, i)
			}
		}
	}
	return nil
}

// NewShowData creates an editable show using the controller's configured names.
func NewShowData(channelNames []string) ShowData {
	tracks := make(map[string]ShowTrack, len(channelNames)+1)
	for _, name := range channelNames {
		tracks[name] = ShowTrack{}
	}
	tracks[BeatTrackID] = ShowTrack{Locked: true}
	return ShowData{Version: ShowFormatVersion, Tracks: tracks}
}
