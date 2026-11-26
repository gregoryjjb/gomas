package gpio

import "fmt"

// mapTrackPins pairs track names and BCM pins by position. A repeated name
// controls every pin at its positions; trailing pins need no track assignment.
func mapTrackPins(pinout []int, tracks []string) (map[string][]int, error) {
	if len(tracks) > len(pinout) {
		return nil, fmt.Errorf("tracks has %d entries but pinout only has %d pins", len(tracks), len(pinout))
	}
	mapping := make(map[string][]int, len(tracks))
	for i, name := range tracks {
		mapping[name] = append(mapping[name], pinout[i])
	}
	return mapping, nil
}
