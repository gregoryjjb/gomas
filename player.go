package main

import (
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/faiface/beep"
	"github.com/faiface/beep/mp3"
	"github.com/faiface/beep/speaker"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
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
}

func NewPlayer() *Player {
	state := &ExternalPlayerState{}
	ch := make(chan PlayerMessage)
	// go workerLoop(ch, state)
	go func() {
		pi, err := newPlayerInternals()
		if err != nil {
			panic(err)
		}
		pi.run(ch)
	}()

	return &Player{
		commandChannel: ch,
		state:          state,
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

//////////////////////////////////
// Keyframe player

// type RenderedKeyframe struct {
// 	time   float64
// 	values []int64
// }

type KeyframePlayer struct {
	frames []*FlatKeyframe
	index  int64
	gpio   *GPIO
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

const speakerBufferSize = time.Millisecond * 200

// Returns true if done
func (kp *KeyframePlayer) Execute(duration time.Duration) (bool, error) {
	// bias := speakerBufferSize.Seconds() - (time.Millisecond * 50).Seconds()
	bias := 0.0
	secs := duration.Seconds() - bias

	if len(kp.frames) <= int(kp.index) {
		return true, nil
	}

	// Hack, make each song short seconds for testing
	if secs >= 5 {
		return true, nil
	}

	current := kp.frames[kp.index]

	if current.Time <= secs {
		if err := kp.gpio.Execute(current.States); err != nil {
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

type AudioPlayer struct {
	streamer beep.StreamCloser
	format   beep.Format
}

func NewAudioPlayer() *AudioPlayer {
	return &AudioPlayer{}
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
	// ap.streamer = streamer
	ap.format = format

	speaker.Init(format.SampleRate, format.SampleRate.N(speakerBufferSize))

	ap.streamer = streamer

	done := make(chan bool)
	speaker.Play(beep.Seq(ap.streamer, beep.Callback(func() {
		done <- true
	})))

	return time.Now(), nil
}

//////////////////////////////////
// State machine

type PlayerState string

const (
	StateIdle    = "idle"
	StatePlaying = "playing"
	StateResting = "resting" // Waiting in between songs
)

const restPeriod = time.Second * 3

type playerInternals struct {
	running   bool
	state     PlayerState
	startTime time.Time
	queue     CircularList[string]

	audioPlayer    *AudioPlayer
	keyframePlayer *KeyframePlayer
}

func newPlayerInternals() (*playerInternals, error) {

	g, err := NewGPIO()
	if err != nil {
		return nil, err
	}

	return &playerInternals{
		state:          StateIdle,
		audioPlayer:    NewAudioPlayer(),
		keyframePlayer: &KeyframePlayer{gpio: g},
	}, nil
}

func (pi *playerInternals) run(channel chan PlayerMessage) {
	if pi.running {
		plog.Fatal().Msg("cannot call run on playerInternals more than once")
	}
	pi.running = true
	plog.Print("running player loop")

	for {
		// Handle incoming message
		select {
		case msg := <-channel:
			plog.Debug().
				Str("command", string(msg.Command)).
				Str("value", msg.Value).
				Msg("received message")

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

		// Handle state change
		switch pi.state {
		case StatePlaying:
			t := time.Since(pi.startTime)
			// t := audioPlayer.CurrentTime()
			done, err := pi.keyframePlayer.Execute(t)
			if err != nil {
				pi.handleError(err)
			} else if done {
				plog.Print("done signal received, ending current show")
				pi.handleShowEnd()
			}

		case StateResting:
			t := time.Since(pi.startTime)
			if t >= restPeriod {
				pi.playNextShow()
			}
		}

		time.Sleep(time.Millisecond)
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
}

func (pi *playerInternals) playShow(id string) error {
	pi.state = StatePlaying
	plog.Info().Str("id", id).Msg("playing show")

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

	return nil
}

func (pi *playerInternals) playNextShow() error {
	pi.clearCurrentShow()
	if pi.queue.Length() > 1 {
		pi.queue.Advance()
		pi.playShow(pi.queue.Current())
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
	pi.playShow(pi.queue.Current())
	return nil
}

func (pi *playerInternals) handleShowEnd() {
	pi.clearCurrentShow()

	if pi.queue.Length() > 1 {
		plog.Info().
			Str("period", restPeriod.String()).
			Str("next_up", pi.queue.PeekNext()).
			Msg("resting")

		pi.state = StateResting
		pi.startTime = time.Now()
	} else {
		// No more items in queue, stop
		pi.enterIdle()
	}
}

func (pi *playerInternals) handleError(err error) {
	if err != nil {
		plog.Err(err).Msg("player error")
		pi.enterIdle()
	}
}
