package main

import (
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/gopxl/beep"
	"github.com/gopxl/beep/mp3"
	"github.com/gopxl/beep/speaker"
)

const sampleRate = beep.SampleRate(48000)

// SpeakerPlayer plays audio through the speakers
type SpeakerPlayer struct {
	streamer beep.StreamSeekCloser
	format   beep.Format
}

var (
	speakerOnce   sync.Once
	speakerErr    error
	speakerMu     sync.Mutex
	speakerBuffer *time.Duration
)

// NewSpeakerPlayer returns a SpeakerPlayer; multiple can be created but
// they must all have the same sample rate (a limitation of Oto)
func NewSpeakerPlayer(buffer time.Duration) (*SpeakerPlayer, error) {
	speakerMu.Lock()
	defer speakerMu.Unlock()

	if speakerBuffer != nil && *speakerBuffer != buffer {
		return nil, fmt.Errorf("cannot initialize speaker with different sample rates (tried %s, already had %s)", buffer, speakerBuffer)
	}
	speakerBuffer = &buffer

	speakerOnce.Do(func() {
		speakerErr = speaker.Init(sampleRate, sampleRate.N(buffer))
	})
	if speakerErr != nil {
		return nil, speakerErr
	}

	return &SpeakerPlayer{}, nil
}

func (ap *SpeakerPlayer) Stop() {
	speaker.Clear()
	if ap.streamer != nil {
		ap.streamer.Close()
	}
}

func (ap *SpeakerPlayer) Play(audio io.ReadCloser) (time.Time, error) {
	// Always stop before playing another!
	ap.Stop()

	streamer, format, err := mp3.Decode(audio)
	if err != nil {
		return time.Time{}, err
	}

	ap.format = format
	ap.streamer = streamer

	if format.SampleRate != sampleRate {
		plog.Debug().Int("from", int(format.SampleRate)).Int("to", int(sampleRate)).Msg("Resampling")
		speaker.Play(beep.Resample(1, format.SampleRate, sampleRate, streamer))
	} else {
		plog.Debug().Msg("Playing directly, no resampling")
		speaker.Play(ap.streamer)
	}

	return time.Now(), nil
}

func (ap *SpeakerPlayer) Close() {
	speaker.Close()
}

// Position _should_ return the position of the audio playback
// but it's choppy so it's not used right now
func (ap *SpeakerPlayer) Position() time.Duration {
	if ap.streamer == nil {
		return 0
	}

	return ap.format.SampleRate.D(ap.streamer.Position())
}
