# Shimmer show format, version 3

A `.shmr` file is a ZIP archive containing `data.json` and `audio.mp3` at its
root. The archive filename without its extension supplies the show name.
Gomas stores these files under `data_dir/songs/<show name>/`.

Gomas supports only version 3. The version is the string `"3"`, not the number
`3`. Unversioned documents and all other versions, including version 2, are
rejected without migration. The Go model and decoder are in
[format.go](format.go); playback is in [player.go](player.go).

## JSON structure

```json
{
  "version": "3",
  "updatedAt": "2026-11-08T12:00:00.000Z",
  "tracks": {
    "Beat": {"locked": true, "keyframes": []},
    "Roof 1": {
      "locked": false,
      "keyframes": [
        {"ts": 0, "value": 0},
        {"ts": 1.5, "value": 1, "selected": true},
        {"ts": 2, "value": 0}
      ]
    }
  }
}
```

- `tracks` is an object keyed by nonblank, case-sensitive channel names.
  There are no `type`, `id`, or `children` fields in a track. Shimmer stores
  layout groups independently of show data; Gomas does not need that layout.
- The map may be empty and channels may be absent. Each present track requires
  a `keyframes` array, which may be empty. Missing or `null` tracks/keyframes
  are rejected. Go nil maps and slices serialize as `{}` and `[]` through
  `encoding/json/v2`.
- `updatedAt` is optional editor timestamp metadata. Gomas preserves the string
  as supplied, omits it when empty, and does not use it for playback or freshness
  checks. Slave synchronization uses a SHA-256 of the stored bytes.
- `locked` is a boolean, defaulting to false when omitted. Gomas always writes
  it, matching Shimmer's track type. `selected` is optional boolean keyframe
  metadata, defaulting to false and omitted when false. Gomas also accepts
  `null` as false for these fields. Neither affects lighting playback.
- Keyframes use numeric `ts` and `value`; Gomas defaults missing or `null`
  values to zero. `ts` is a finite, nonnegative number of seconds from the start
  of the audio. `value` is finite; positive values mean on, and zero or negative
  values mean off. Timestamps must be in ascending order; equal timestamps
  are allowed. Gomas rejects unsorted keyframes rather than sorting them.
- The reserved channel `Beat` identifies editor beat markers. It may be absent.
  Beat data is saved and exported, but never drives GPIO or extends playback.
- Unknown fields are ignored by the decoder and are not retained when saving
  the parsed show. Duplicate JSON object keys and trailing JSON values are rejected.

## GPIO mapping and playback

The controller's TOML config maps exact channel names to BCM GPIO pin numbers:

```toml
[pinout]
"Roof 1" = 17
"Roof 2" = 27
```

Object order does not control wiring. Each controller plays only the channels
in its `pinout`; it can also play audio with no matching channels. Duplicate
GPIO pin assignments and mappings for `Beat` are rejected. Channel names are
read from config when a show is loaded. Physical GPIO pins are initialized at
startup, so restart after changing the pinout.

All mapped channels start off. Each keyframe sets a state that persists until
another keyframe changes it. Each tick catches up through all due keyframes,
then sends the final changed channel states to GPIO.

Duration is the latest keyframe timestamp across all lighting tracks, including
unmapped tracks but excluding Beat. Playback ends one second after that time.
If all lighting tracks are absent or empty, the duration is one second.
The configured `bias` delays the lighting timeline relative to audio. Idle and
rest settings determine what the lights do between shows.

## Editing and storage

Disk reads, API updates, and archive imports use the same decoder. Saves validate
the show before writing `data.json`. Exports retain supported editor metadata and
include the audio unchanged. New shows contain an empty Beat track and the
controller's configured channel names. Although empty track maps are valid v3 data,
the show-data PUT endpoint rejects them to prevent accidentally overwriting a show.

`GET /api/shows/{name}/data?raw=true` returns the stored bytes unchanged.
Slaves verify the downloaded hash and format before saving the raw bytes.

[testdata/show-v3.json](testdata/show-v3.json) covers named channels, empty tracks,
locking, selection, updated timestamps, and Beat markers.
