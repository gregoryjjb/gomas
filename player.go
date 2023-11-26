package main

import (
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/gopxl/beep"
	"github.com/gopxl/beep/mp3"
	"github.com/gopxl/beep/speaker"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"gregoryjjb/gomas/gpio"
	"gregoryjjb/gomas/pubsub"
)

var plog zerolog.Logger

func init() {
	plog = log.With().Str("component", "player").Logger()
}

type PlayerCommand string

const (
	CommandPlay    PlayerCommand = "play"
	CommandPlayAll PlayerCommand = "playall"
	CommandStop    PlayerCommand = "stop"
	CommandNext    PlayerCommand = "next"
)

type PlayerEvent struct {
	State   PlayerState `json:"state"`
	Payload string      `json:"payload"`
}

type ExternalPlayerState struct {
	mutex sync.RWMutex
	value string
}

func (eps *ExternalPlayerState) Get() string {
	eps.mutex.RLock()
	defer eps.mutex.RUnlock()

	return eps.value
}

func (eps *ExternalPlayerState) Set(v string) {
	eps.mutex.Lock()
	defer eps.mutex.Unlock()

	eps.value = v
}

type PlayerMessage struct {
	Command PlayerCommand
	Value   string
}

type Player struct {
	commandChannel chan PlayerMessage
	state          *ExternalPlayerState
	pubsub         *pubsub.Pubsub[PlayerEvent]
}

func NewPlayer() *Player {
	state := &ExternalPlayerState{}
	ch := make(chan PlayerMessage)
	// go workerLoop(ch, state)
	pi, err := newPlayerInternals()
	if err != nil {
		panic(err)
	}
	go pi.run(ch)

	return &Player{
		commandChannel: ch,
		state:          state,
		pubsub:         pi.ps,
	}
}

func (p *Player) Play(id string) {
	p.commandChannel <- PlayerMessage{
		Command: CommandPlay,
		Value:   id,
	}
}

func (p *Player) PlayAll() {
	p.commandChannel <- PlayerMessage{
		Command: CommandPlayAll,
	}
}

func (p *Player) Stop() {
	p.commandChannel <- PlayerMessage{
		Command: CommandStop,
	}
}

func (p *Player) Next() {
	p.commandChannel <- PlayerMessage{
		Command: CommandNext,
	}
}

func (p *Player) Subscribe() (func(), <-chan PlayerEvent) {
	handle, ch := p.pubsub.Subscribe()
	return func() {
		p.pubsub.Unsubscribe(handle)
	}, ch
}

//////////////////////////////////
// Keyframe player

// type RenderedKeyframe struct {
// 	time   float64
// 	values []int64
// }

type KeyframePlayer struct {
	frames []*FlatKeyframe
	index  int64
	bias   time.Duration
}

func (kp *KeyframePlayer) Load(id string) error {
	kfs, err := LoadFlatKeyframes(id)

	if err != nil {
		return err
	}

	plog.Debug().Int("length", len(kfs)).Msg("loaded keyframes")

	kp.frames = kfs
	return nil
}

// Returns true if done
func (kp *KeyframePlayer) Execute(duration time.Duration) (bool, error) {
	bias := kp.bias
	secs := (duration - bias).Seconds()

	if len(kp.frames) <= int(kp.index) {
		return true, nil
	}

	current := kp.frames[kp.index]

	if current.Time <= secs {
		if err := gpio.Execute(current.States); err != nil {
			return false, err
		}
		kp.index += 1
	}

	return false, nil
}

func (kp *KeyframePlayer) Unload() {
	kp.frames = nil
	kp.index = 0
}

//////////////////////////////////
// Audio player

const sampleRate = beep.SampleRate(48000)

type AudioPlayer struct {
	streamer beep.StreamSeekCloser
	format   beep.Format
}

func NewAudioPlayer() *AudioPlayer {
	return &AudioPlayer{}
}

func (ap *AudioPlayer) Init() error {
	buff := GetConfigSpeakerBuffer()
	return speaker.Init(sampleRate, sampleRate.N(buff))
}

func (ap *AudioPlayer) Stop() {
	speaker.Clear()
	if ap.streamer != nil {
		ap.streamer.Close()
	}
}

func (ap *AudioPlayer) Play(path string) (time.Time, error) {
	// Always stop before playing another!
	ap.Stop()

	f, err := os.Open(path)
	if err != nil {
		return time.Time{}, err
	}

	streamer, format, err := mp3.Decode(f)
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

// Position _should_ return the position of the audio playback
// but it's choppy so it's not used right now
func (ap *AudioPlayer) Position() time.Duration {
	if ap.streamer == nil {
		return 0
	}

	return ap.format.SampleRate.D(ap.streamer.Position())
}

//////////////////////////////////
// State machine

type PlayerState string

const (
	StateIdle    = "idle"
	StatePlaying = "playing"
	StateResting = "resting" // Waiting in between songs
)

type playerInternals struct {
	running   bool
	state     PlayerState
	startTime time.Time
	queue     CircularList[string]

	audioPlayer    *AudioPlayer
	keyframePlayer *KeyframePlayer

	ps *pubsub.Pubsub[PlayerEvent]
}

func newPlayerInternals() (*playerInternals, error) {
	ap := NewAudioPlayer()
	plog.Debug().Msg("Initializing speakers")
	if err := ap.Init(); err != nil {
		return nil, err
	}

	return &playerInternals{
		state:          StateIdle,
		audioPlayer:    ap,
		keyframePlayer: &KeyframePlayer{bias: GetConfigBias()},
		ps:             pubsub.New[PlayerEvent](),
	}, nil
}

func (pi *playerInternals) run(channel chan PlayerMessage) {
	if pi.running {
		plog.Fatal().Msg("Cannot call run on playerInternals more than once")
	}
	pi.running = true
	plog.Print("Running player loop")

	for {
		// Handle incoming message
		select {
		case msg := <-channel:
			plog.Debug().
				Str("command", string(msg.Command)).
				Str("value", msg.Value).
				Msg("Received message")

			switch msg.Command {
			case CommandPlay:
				pi.clearCurrentShow()
				pi.clearQueue()
				pi.handleError(pi.playShow(msg.Value))

			case CommandPlayAll:
				pi.clearCurrentShow()
				pi.clearQueue()
				pi.handleError(pi.playAllShows())

			case CommandStop:
				pi.enterIdle()

			case CommandNext:
				pi.handleError(pi.playNextShow())
			}
		default:
			// Do nothing
		}

		// Handle actions required by current state
		switch pi.state {
		case StatePlaying:
			t := time.Since(pi.startTime)
			done, err := pi.keyframePlayer.Execute(t)
			if err != nil {
				pi.handleError(err)
			} else if done {
				plog.Print("Done signal received, ending current show")
				pi.handleShowEnd()
			}

		case StateResting:
			t := time.Since(pi.startTime)
			if t >= GetConfigRestPeriod() {
				pi.playNextShow()
			}
		}

		fps := GetConfigFramesPerSecond()
		delay := time.Second / time.Duration(fps)
		time.Sleep(delay)
	}
}

func (pi *playerInternals) clearCurrentShow() {
	pi.audioPlayer.Stop()
	pi.keyframePlayer.Unload()
}

func (pi *playerInternals) clearQueue() {
	pi.queue.Clear()
}

func (pi *playerInternals) enterIdle() {
	pi.clearCurrentShow()
	pi.clearQueue()
	pi.state = StateIdle
	pi.ps.Publish(PlayerEvent{
		State: StateIdle,
	})
}

func (pi *playerInternals) playShow(id string) error {
	plog.Info().Str("id", id).Msg("Playing show")

	err := pi.keyframePlayer.Load(id)
	if err != nil {
		return err
	}

	audioPath, err := ShowAudioPath(id)
	if err != nil {
		return err
	}

	pi.startTime, err = pi.audioPlayer.Play(audioPath)
	if err != nil {
		return err
	}

	pi.state = StatePlaying
	pi.ps.Publish(PlayerEvent{
		State:   StatePlaying,
		Payload: id,
	})

	return nil
}

func (pi *playerInternals) playNextShow() error {
	pi.clearCurrentShow()
	if pi.queue.Length() > 1 {
		pi.queue.Advance()
		if err := pi.playShow(pi.queue.Current()); err != nil {
			return err
		}
	} else {
		pi.enterIdle()
	}
	return nil
}

func (pi *playerInternals) playAllShows() error {
	shows, err := ListShows()
	if err != nil {
		return fmt.Errorf("cannot play all: %e", err)
	}

	var ids []string
	for _, show := range shows {
		if show.HasAudio {
			ids = append(ids, show.ID)
		}
	}

	if len(ids) == 0 {
		return errors.New("cannot play all: no playable shows found")
	}

	pi.queue.Replace(ids)
	if err := pi.playShow(pi.queue.Current()); err != nil {
		return err
	}
	return nil
}

func (pi *playerInternals) handleShowEnd() {
	pi.clearCurrentShow()

	if pi.queue.Length() > 1 {
		plog.Info().
			Str("period", GetConfigRestPeriod().String()).
			Str("next_up", pi.queue.PeekNext()).
			Msg("Resting")

		pi.state = StateResting
		pi.ps.Publish(PlayerEvent{
			State:   StateResting,
			Payload: pi.queue.PeekNext(),
		})
		pi.startTime = time.Now()
	} else {
		// No more items in queue, stop
		pi.enterIdle()
	}
}

func (pi *playerInternals) handleError(err error) {
	if err != nil {
		plog.Err(err).Msg("Player error")
		pi.enterIdle()
	}
}
