package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"gregoryjjb/gomas/gpio"
	"gregoryjjb/gomas/pubsub"
)

var plog = log.With().Str("component", "player").Logger()

type PlayerCommand string

const (
	CommandPlayAll PlayerCommand = "playall"
	CommandStop    PlayerCommand = "stop"
	CommandNext    PlayerCommand = "next"
)

func (pc PlayerCommand) String() string {
	return string(pc)
}

type commandPlay struct {
	id        string
	startedAt time.Time
}

func (c commandPlay) String() string {
	return fmt.Sprintf("play(%q, %v)", c.id, c.startedAt)
}

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

type PlayerMessage interface {
	fmt.Stringer
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
	p.commandChannel <- commandPlay{
		id: id,
	}
}

func (p *Player) PlaySlave(id string, startTime time.Time) {
	p.commandChannel <- commandPlay{
		id:        id,
		startedAt: startTime,
	}
}

func (p *Player) PlayAll() {
	p.commandChannel <- CommandPlayAll
}

func (p *Player) Stop() {
	p.commandChannel <- CommandStop
}

func (p *Player) Next() {
	p.commandChannel <- CommandNext
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
	runOnce sync.Once

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

	for _, slaveHost := range config.Slaves() {
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

func (pi *playerInternals) run(ctx context.Context, ch chan PlayerMessage) {
	pi.runOnce.Do(func() {
		for {
			if err := ctx.Err(); err != nil {
				plog.Error().Err(err).Msg("Aborting player")
				pi.audio.Stop()
				pi.audio.Close()
				return
			}

			if err := pi.loopIteration(ch); err != nil {
				plog.Err(err).Msg("Player loop encountered an error")
				pi.enterIdle()
			}

			fps := pi.config.FramesPerSecond()
			delay := time.Second / time.Duration(fps)
			time.Sleep(delay)
		}
	})
}

func (pi *playerInternals) loopIteration(ch chan PlayerMessage) error {
	// Handle incoming message
	select {
	case msg := <-ch:
		plog.Info().Stringer("command", msg).Msg("Received message")

		switch msg := msg.(type) {
		case commandPlay:
			pi.clearCurrentShow()
			pi.clearQueue()
			if err := pi.playShow(msg.id, msg.startedAt); err != nil {
				return fmt.Errorf("playShow: %w", err)
			}

		case PlayerCommand:
			switch msg {
			case CommandPlayAll:
				pi.clearCurrentShow()
				pi.clearQueue()
				if err := pi.playAllShows(); err != nil {
					return fmt.Errorf("playAllShows: %w", err)
				}

			case CommandStop:
				pi.enterIdle()

			case CommandNext:
				if err := pi.playNextShow(); err != nil {
					return fmt.Errorf("playNextShow: %w", err)
				}
			}
		default:
			return fmt.Errorf("received invalid message: %v", msg)
		}
	default:
		// Do nothing, no messages to receive
	}

	// Handle actions required by current state
	switch pi.state {
	case StatePlaying:
		done, err := pi.executeKeyframe()
		if err != nil {
			return err
		} else if done {
			plog.Print("End of current show keyframes reached")
			pi.handleShowEnd()
		}

	case StateResting:
		t := time.Since(pi.startTime)
		if t >= pi.config.RestPeriod() {
			pi.playNextShow()
		}
	}

	return nil
}

// executeKeyframe wirtes a keyframe to gpio based on
// how much time has passed since the start
func (pi *playerInternals) executeKeyframe() (bool, error) {
	bias := pi.config.Bias()
	t := time.Since(pi.startTime)
	secs := (t - bias).Seconds()

	if pi.keyframeIndex >= len(pi.keyframes) {
		// Keyframes are finished

		// Wait for an additional 1 second before ending this song
		last := pi.keyframes[len(pi.keyframes)-1]
		elapsedBuffer := secs - last.Time
		extraBuffer := 1.0

		return elapsedBuffer >= extraBuffer, nil
	}

	next := pi.keyframes[pi.keyframeIndex]
	if next.Time <= secs {
		if err := gpio.Execute(next.States[pi.config.ChannelOffset():]); err != nil {
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

func (pi *playerInternals) playShow(id string, startedAt time.Time) error {
	data, err := pi.storage.ReadShowData(id)
	if err != nil {
		return err
	}
	pi.keyframes = data.FlatKeyframes()
	plog.Debug().Int("keyframe_count", len(pi.keyframes)).Msg("Loaded keyframes")
	if len(pi.keyframes) == 0 {
		return fmt.Errorf("show %q had zero keyframes", id)
	}

	offset := pi.config.ChannelOffset()

	if offset >= len(pi.keyframes[0].States) {
		plog.Warn().
			Int("channel_offset", offset).
			Int("actual_channel_count", len(pi.keyframes[0].States)).
			Msg("Configured channel offset will cause no keyframes to be played")
	}

	if !startedAt.IsZero() {
		pi.startTime = startedAt
	} else {
		audio, err := pi.storage.ReadAudio(id)
		if err != nil {
			return err
		}

		pi.startTime, err = pi.audio.Play(audio)
		if err != nil {
			return err
		}

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

	plog.Info().
		Str("id", id).
		Time("start_time", pi.startTime).
		Bool("slave", !startedAt.IsZero()).
		Msg("Started playing show")

	return nil
}

func (pi *playerInternals) playNextShow() error {
	pi.clearCurrentShow()
	if pi.queue.Length() > 1 {
		pi.queue.Advance()
		if err := pi.playShow(pi.queue.Current(), time.Time{}); err != nil {
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
	if err := pi.playShow(pi.queue.Current(), time.Time{}); err != nil {
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

type Slaves struct {
	hosts []url.URL
}

func (s *Slaves) send(u string) {
	if _, err := http.Get(u); err != nil {
		plog.Err(err).Msg("Failed to notify slave")
	}
}

func (s *Slaves) Play(id string, startTime time.Time) {
	for _, u := range s.hosts {

		go func() {
			q := make(url.Values)
			q.Set("slaveStartTimeMicro", MarshalStartTime(startTime))

			u.Path += "/api/play/" + id
			u.RawQuery = q.Encode()

			s.send(u.String())
		}()
	}
}

func (s *Slaves) Stop() {
	for _, u := range s.hosts {

		go func() {
			u.Path += "/api/static"

			s.send(u.String())
		}()
	}
}

func MarshalStartTime(t time.Time) string {
	return strconv.FormatInt(t.UnixMicro(), 10)
}

func UnmarshalStartTime(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}

	micros, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return time.Time{}, err
	}

	return time.UnixMicro(micros), nil
}
