package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
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
	internals      *playerInternals
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
		internals:      pi,
	}
}

func (p *Player) Play(id string) {
	p.commandChannel <- PlayerMessage{
		Command: CommandPlay,
		Value:   "#" + id,
	}
}

func (p *Player) PlaySlave(id string, startTime int64) {
	p.commandChannel <- PlayerMessage{
		Command: CommandPlay,
		Value:   fmt.Sprintf("%d#%s", startTime, id),
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

func (p *Player) State() *SongState {
	p.internals.songStateMu.RLock()
	defer p.internals.songStateMu.RUnlock()

	if p.internals.songState == nil {
		return nil
	}

	s := *p.internals.songState
	return &s
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

type SongState struct {
	ID        string
	StartedAt time.Time
}

type playerInternals struct {
	storage *Storage
	config  *Config
	audio   AudioPlayer
	slaves  *Slaves

	running       bool
	state         PlayerState
	startTime     time.Time
	queue         CircularList[string]
	keyframes     []FlatKeyframe
	keyframeIndex int

	ps          *pubsub.Pubsub[PlayerEvent]
	songState   *SongState
	songStateMu sync.RWMutex
}

func newPlayerInternals(config *Config, storage *Storage, audio AudioPlayer) (*playerInternals, error) {
	slaves := &Slaves{}

	for _, slaveHost := range config.toml.Slaves {
		u, err := url.Parse(slaveHost)
		if err != nil {
			return nil, fmt.Errorf("invalid slave host %q in config: %w", slaveHost, err)
		}
		slaves.hosts = append(slaves.hosts, *u)
	}

	return &playerInternals{
		storage: storage,
		config:  config,
		audio:   audio,
		slaves:  slaves,

		state: StateIdle,

		ps: pubsub.New[PlayerEvent](),
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
		last := pi.keyframes[len(pi.keyframes)-1]
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
	pi.slaves.Stop()

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
	pi.songStateMu.Lock()
	defer pi.songStateMu.Unlock()
	pi.songState = nil
}

func (pi *playerInternals) playShow(payload string) error {
	plog.Info().Str("payload", payload).Msg("Playing show")

	elements := strings.SplitN(payload, "#", 2)
	if len(elements) != 2 {
		return fmt.Errorf("could not parse %q as a play payload", payload)
	}

	id := elements[1]

	data, err := pi.storage.ReadShowData(id)
	if err != nil {
		return err
	}
	pi.keyframes = data.FlatKeyframes()
	plog.Debug().Int("keyframe_count", len(pi.keyframes)).Msg("Loaded keyframes")
	if len(pi.keyframes) == 0 {
		return fmt.Errorf("show %q had zero keyframes", id)
	}

	if elements[0] != "" {
		startTimeMicro, err := strconv.ParseInt(elements[0], 10, 64)
		if err != nil {
			return fmt.Errorf("parse start time: %w", err)
		}

		pi.startTime = time.UnixMicro(startTimeMicro)
	} else {
		audio, err := pi.storage.ReadAudio(id)
		if err != nil {
			return err
		}

		pi.startTime, err = pi.audio.Play(audio)
		if err != nil {
			return err
		}

		// for _, slave := range pi.config.toml.Slaves {
		// 	plog.Info().Str("slave_host", slave).Str("id", id).Int64("start_time", pi.startTime.UnixMicro()).Msg("Notifying slave")
		// 	go notifySlave(slave, id, pi.startTime.UnixMicro())
		// }

		pi.slaves.Play(id, pi.startTime)
	}

	pi.state = StatePlaying
	pi.ps.Publish(PlayerEvent{
		State:   StatePlaying,
		Payload: id,
	})
	pi.songStateMu.Lock()
	defer pi.songStateMu.Unlock()
	pi.songState = &SongState{
		ID:        id,
		StartedAt: pi.startTime,
	}

	fmt.Println("Start time:", pi.startTime.UnixMicro())

	return nil
}

func (pi *playerInternals) playNextShow() error {
	pi.clearCurrentShow()
	if pi.queue.Length() > 1 {
		pi.queue.Advance()
		if err := pi.playShow("#" + pi.queue.Current()); err != nil {
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
	if err := pi.playShow("#" + pi.queue.Current()); err != nil {
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

type Slaves struct {
	hosts []url.URL
}

func (s *Slaves) Play(id string, startTime time.Time) {
	for _, u := range s.hosts {

		go func() {
			q := make(url.Values)
			q.Set("slaveStartTimeMicro", strconv.FormatInt(startTime.UnixMicro(), 10))

			u.Path = "/api/play/" + id
			u.RawQuery = q.Encode()

			if _, err := http.Get(u.String()); err != nil {
				fmt.Println("ERROR NOTIFYING SLAVE: %s", err)
			}

		}()
	}
}

func (s *Slaves) Stop() {
	for _, u := range s.hosts {

		go func() {
			u.Path = "/api/static"

			if _, err := http.Get(u.String()); err != nil {
				fmt.Println("ERROR NOTIFYING SLAVE: %s", err)
			}
		}()
	}
}
