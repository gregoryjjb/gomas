# Shimmer show format, version 2

A `.shmr` file is a ZIP archive containing `data.json` and `audio.mp3` at its
root. The archive filename without its extension supplies the show name.
Gomas stores these files under `data_dir/songs/<show name>/`.

Gomas supports only version 2. The version is the string `"2"`, not the number
`2`. Unversioned documents and other versions are rejected. The Go model and
decoder are in [format.go](format.go); playback is in [player.go](player.go).

## JSON structure

```json
{
  "version": "2",
  "tracks": [
    {"type": "track", "id": "Beat", "locked": true, "keyframes": []},
    {
      "type": "group",
      "id": "Roof parkview",
      "children": [
        {
          "type": "track",
          "id": "Roof 1",
          "locked": false,
          "keyframes": [
            {"ts": 0, "value": 0},
            {"ts": 1.5, "value": 1, "selected": true},
            {"ts": 2, "value": 0}
          ]
        }
      ]
    }
  ]
}
```

- `tracks` is an array of track or group nodes. Groups can contain both kinds
  of node recursively through `children`.
- Every node has a nonempty, case-sensitive `id`, unique across the entire
  document, including groups. The ID is also its display name in Shimmer.
- Track nodes require `type: "track"`, `id`, and a `keyframes` array.
  `locked` is an optional boolean, defaulting to false; Gomas omits it when false.
- Group nodes require `type: "group"`, `id`, and a `children` array.
- Keyframes use numeric `ts` and `value`; Gomas defaults missing or `null` values
  to zero. `ts` is a finite, nonnegative
  number of seconds from the start of the audio. `value` is finite; positive
  values mean on, and zero or negative values mean off.
- `selected` is optional boolean editor metadata, defaulting to false and omitted when false.
  Gomas also accepts `null` as false for both boolean fields.
  `locked` and `selected` do not affect lighting playback.
- The reserved track ID `Beat` identifies editor beat markers. It may be
  absent, but a group cannot use that ID. Beat data is saved and exported, but
  never drives GPIO.
- Empty arrays are valid. Required arrays cannot be missing or `null`.
  Go nil slices serialize as `[]` through `encoding/json/v2`.
- Unknown fields are ignored when decoding and are not retained when Gomas saves
  the decoded show. Unknown node types, duplicate JSON object keys, and trailing
  JSON values are rejected.

## GPIO mapping and playback

The controller's TOML config maps exact track IDs to BCM GPIO pin numbers:

```toml
[pinout]
"Roof 1" = 17
"Roof 2" = 27
```

Groups and document order do not control wiring. Each controller maps only its
own lighting tracks. The player loads tracks whose IDs appear in `pinout` and
requires at least one matching track. Duplicate GPIO pin assignments and mappings
for `Beat` are rejected. Channel names are read from config when a show is loaded.
Physical GPIO pins are initialized at startup, so restart after
changing the pinout. Positional pin arrays and `channel_offset` are no longer used.

All mapped channels start off. Each keyframe sets a state that persists until
another keyframe changes it. Playback reads each mapped track's keyframes in
array order, so timestamps should be sorted. Each tick applies at most one due
keyframe per track, sending only updated channel states to GPIO.

The player calculates duration from the last array element of every nonempty
lighting track, including unmapped tracks but excluding Beat markers. Playback
ends one second after the latest of those timestamps. If all lighting tracks are empty, the
duration is one second. This is a keyframe-based duration, not the duration of
the MP3. The configured `bias`
delays the lighting timeline relative to audio. Existing idle and rest settings
continue to determine what the lights do between shows.

## Editing and storage

Disk reads, API updates, and archive imports use the same decoder. Saves validate
the whole tree before writing `data.json`. Exports retain editor metadata and
include the audio unchanged. New shows contain an empty Beat track
and the controller's configured channel names; Shimmer can organize those tracks
into groups.

[testdata/show-v2.json](testdata/show-v2.json) is a small test fixture covering
nested groups, empty tracks, locking, selection, and Beat markers.
