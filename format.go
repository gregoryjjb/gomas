package main

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
	"io"
	"math"
)

// ShowFormatVersion is the version string stored in data.json.
const ShowFormatVersion = "2"

// BeatTrackID is reserved for editor beat markers and never drives GPIO.
const BeatTrackID = "Beat"

// ShowData is data.json in a .shmr ZIP alongside audio.mp3. The archive filename
// supplies the show name. Unversioned shows are not supported.
type ShowData struct {
	Version string    `json:"version"`
	Tracks  ShowNodes `json:"tracks"`
}

// ShowNode is either a *ShowTrack or *ShowGroup. IDs are globally unique names.
type ShowNode interface{ isShowNode() }
type ShowNodeType string

const (
	ShowNodeTrack ShowNodeType = "track"
	ShowNodeGroup ShowNodeType = "group"
)

// ShowTrack.ID is its Shimmer name and GPIO mapping key. Locked only affects editing.
type ShowTrack struct {
	Type      ShowNodeType   `json:"type"`
	ID        string         `json:"id"`
	Keyframes []ShowKeyframe `json:"keyframes"`
	Locked    bool           `json:"locked,omitzero"`
}

func (*ShowTrack) isShowNode() {}

// ShowGroup organizes tracks and groups without consuming a GPIO output.
type ShowGroup struct {
	Type     ShowNodeType `json:"type"`
	ID       string       `json:"id"`
	Children ShowNodes    `json:"children"`
}

func (*ShowGroup) isShowNode() {}

// ShowKeyframe sets a value at a time in seconds from the start of the audio.
// Positive values mean on. Selected is editor metadata, not playback state.
type ShowKeyframe struct {
	Timestamp float64 `json:"ts"`
	Value     float64 `json:"value"`
	Selected  bool    `json:"selected,omitzero"`
}

// ShowNodes decodes the recursive track/group union without losing children.
type ShowNodes []ShowNode

func (nodes *ShowNodes) UnmarshalJSONFrom(dec *jsontext.Decoder) error {
	var raw []jsontext.Value
	if err := json.UnmarshalDecode(dec, &raw); err != nil {
		return err
	}
	if raw == nil {
		return fmt.Errorf("nodes must be an array, not null")
	}
	result := make(ShowNodes, 0, len(raw))
	for i, value := range raw {
		var tag struct {
			Type ShowNodeType `json:"type"`
		}
		if err := json.Unmarshal(value, &tag); err != nil {
			return fmt.Errorf("node %d: %w", i, err)
		}
		var node ShowNode
		switch tag.Type {
		case ShowNodeTrack:
			track := &ShowTrack{}
			if err := json.Unmarshal(value, track); err != nil {
				return fmt.Errorf("track %d: %w", i, err)
			}
			if track.Keyframes == nil {
				return fmt.Errorf("track %q: keyframes must be an array", track.ID)
			}

			node = track
		case ShowNodeGroup:
			group := &ShowGroup{}
			if err := json.Unmarshal(value, group); err != nil {
				return fmt.Errorf("group %d: %w", i, err)
			}
			if group.Children == nil {
				return fmt.Errorf("group %q: children must be an array", group.ID)
			}
			node = group
		default:
			return fmt.Errorf("node %d: unsupported type %q", i, tag.Type)
		}
		result = append(result, node)
	}
	*nodes = result
	return nil
}

func (data *ShowData) UnmarshalJSONFrom(dec *jsontext.Decoder) error {
	type plain ShowData
	var result plain
	if err := json.UnmarshalDecode(dec, &result); err != nil {
		return err
	}
	if result.Tracks == nil {
		return fmt.Errorf("tracks must be an array")
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

// Validate checks the format and cross-node constraints before saving or playing.
// Nil slices in Go are valid: JSON v2 writes them as empty arrays.
func (data ShowData) Validate() error {
	if data.Version != ShowFormatVersion {
		return fmt.Errorf("unsupported show version %q (expected %q)", data.Version, ShowFormatVersion)
	}
	seen := make(map[string]bool)
	var visit func(ShowNodes) error
	visit = func(nodes ShowNodes) error {
		for _, node := range nodes {
			var id string
			switch n := node.(type) {
			case *ShowTrack:
				if n == nil || n.Type != ShowNodeTrack {
					return fmt.Errorf("invalid track node")
				}
				id = n.ID
				for i, kf := range n.Keyframes {
					if math.IsNaN(kf.Timestamp) || math.IsInf(kf.Timestamp, 0) || kf.Timestamp < 0 || math.IsNaN(kf.Value) || math.IsInf(kf.Value, 0) {
						return fmt.Errorf("track %q keyframe %d: expected nonnegative finite ts and finite value", id, i)
					}
					if i > 0 && kf.Timestamp < n.Keyframes[i-1].Timestamp {
						return fmt.Errorf("track %q keyframe %d: keyframes must be in ascending order by timestamp", id, i)
					}
				}
			case *ShowGroup:
				if n == nil || n.Type != ShowNodeGroup {
					return fmt.Errorf("invalid group node")
				}
				id = n.ID
				if id == BeatTrackID {
					return fmt.Errorf("%q is reserved for the beat track", id)
				}
			default:
				return fmt.Errorf("invalid show node")
			}
			if id == "" {
				return fmt.Errorf("node ID must not be empty")
			}
			if seen[id] {
				return fmt.Errorf("duplicate node ID %q", id)
			}
			seen[id] = true
			if group, ok := node.(*ShowGroup); ok {
				if err := visit(group.Children); err != nil {
					return err
				}
			}
		}
		return nil
	}
	return visit(data.Tracks)
}

// NewShowData creates an editable show using the controller's configured names.
func NewShowData(channelNames []string) ShowData {
	nodes := ShowNodes{&ShowTrack{Type: ShowNodeTrack, ID: BeatTrackID, Locked: true}}
	for _, name := range channelNames {
		nodes = append(nodes, &ShowTrack{Type: ShowNodeTrack, ID: name})
	}
	return ShowData{Version: ShowFormatVersion, Tracks: nodes}
}
