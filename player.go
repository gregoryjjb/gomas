package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

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

func NewPlayer(ctx context.Context, config *Config, storage *Storage, audio AudioPlayer) *Player {
	state := &ExternalPlayerState{}
	ch := make(chan PlayerMessage)
	// go workerLoop(ch, state)
	pi, err := newPlayerInternals(config, storage, audio)
	if err != nil {
		panic(err)
	}
	go pi.run(ctx, ch)

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
// State machine

type AudioPlayer interface {
	Play(io.ReadCloser) (time.Time, error)
	Stop()
	Close()
}

type PlayerState string

const (
	StateIdle    = "idle"
	StatePlaying = "playing"
	StateResting = "resting" // Waiting in between songs
)

type playerInternals struct {
	storage *Storage
	config  *Config
	audio   AudioPlayer

	running       bool
	state         PlayerState
	startTime     time.Time
	queue         CircularList[string]
	keyframes     []FlatKeyframe
	keyframeIndex int

	ps *pubsub.Pubsub[PlayerEvent]
}

func newPlayerInternals(config *Config, storage *Storage, audio AudioPlayer) (*playerInternals, error) {
	return &playerInternals{
		state:   StateIdle,
		audio:   audio,
		ps:      pubsub.New[PlayerEvent](),
		storage: storage,
		config:  config,
	}, nil
}

func (pi *playerInternals) run(ctx context.Context, channel chan PlayerMessage) {
	if pi.running {
		plog.Fatal().Msg("Cannot call playerInternals.run more than once")
	}
	pi.running = true
	plog.Print("Running player loop")

	for {
		if ctx.Err() != nil {
			plog.Info().Msg("Aborting player")
			pi.audio.Stop()
			pi.audio.Close()
			return
		}

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
			// t := time.Since(pi.startTime)
			done, err := pi.executeKeyframe()
			if err != nil {
				pi.handleError(err)
			} else if done {
				plog.Print("Done signal received, ending current show")
				pi.handleShowEnd()
			}

		case StateResting:
			t := time.Since(pi.startTime)
			if t >= pi.config.RestPeriod() {
				pi.playNextShow()
			}
		}

		fps := pi.config.FramesPerSecond()
		delay := time.Second / time.Duration(fps)
		time.Sleep(delay)
	}
}

// executeKeyframe wirtes a keyframe to gpio based on
// how much time has passed since the start
func (pi *playerInternals) executeKeyframe() (bool, error) {
	bias := pi.config.Bias()
	t := time.Since(pi.startTime)
	secs := (t - bias).Seconds()

	if len(pi.keyframes) <= pi.keyframeIndex {
		// Keyframes are finished

		// Wait for an additional 1 second before ending this song
		last := pi.keyframes[len(pi.keyframes) - 1]
		elapsedBuffer := secs - last.Time
		extraBuffer := 1.0

		return elapsedBuffer >= extraBuffer, nil
	}

	next := pi.keyframes[pi.keyframeIndex]
	if next.Time <= secs {
		if err := gpio.Execute(next.States); err != nil {
			return false, err
		}
		pi.keyframeIndex += 1
	}

	return false, nil
}

func (pi *playerInternals) clearCurrentShow() {
	pi.audio.Stop()
	pi.keyframes = nil
	pi.keyframeIndex = 0
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

	data, err := pi.storage.ReadShowData(id)
	if err != nil {
		return err
	}
	pi.keyframes = data.FlatKeyframes()
	plog.Debug().Int("keyframe_count", len(pi.keyframes)).Msg("Loaded keyframes")
	if len(pi.keyframes) == 0 {
		return fmt.Errorf("show %q had zero keyframes", id)
	}

	audio, err := pi.storage.ReadAudio(id)
	if err != nil {
		return err
	}

	pi.startTime, err = pi.audio.Play(audio)
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
	shows, err := pi.storage.ListShows()
	if err != nil {
		return fmt.Errorf("cannot play all: %e", err)
	}
	if len(shows) == 0 {
		return errors.New("cannot play all: no playable shows found")
	}

	pi.queue.Replace(shows)
	if err := pi.playShow(pi.queue.Current()); err != nil {
		return err
	}
	return nil
}

func (pi *playerInternals) handleShowEnd() {
	pi.clearCurrentShow()

	if pi.queue.Length() > 1 {
		plog.Info().
			Str("period", pi.config.RestPeriod().String()).
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
