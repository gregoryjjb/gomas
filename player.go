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
	"github.com/spf13/afero"
)

// A simple command consists only of a command name with no variable information
type simpleCommand string

const (
	CommandPlayAll simpleCommand = "playall"
	CommandNext    simpleCommand = "next"
)

func (pc simpleCommand) String() string {
	return string(pc)
}

type commandPlay struct {
	id        string
	startedAt time.Time
}

func (c commandPlay) String() string {
	return fmt.Sprintf("play(%q, %v)", c.id, c.startedAt)
}

type commandStatic bool

func (c commandStatic) String() string {
	s := "off"
	if c {
		s = "on"
	}

	return fmt.Sprintf("static(%s)", s)
}

type PlayerMessage interface {
	fmt.Stringer
}

type Player struct {
	commandChannel chan PlayerMessage
	internals      *playerInternals
}

func NewPlayer(ctx context.Context, config *Config, storage PlayerStorage, audio AudioPlayer, gpio GPIO) *Player {
	ch := make(chan PlayerMessage)
	// go workerLoop(ch, state)
	pi, err := newPlayerInternals(config, storage, audio, gpio)
	if err != nil {
		panic(err)
	}
	go pi.run(ctx, ch)

	return &Player{
		commandChannel: ch,
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

func (p *Player) Static(state bool) {
	p.commandChannel <- commandStatic(state)
}

func (p *Player) Next() {
	p.commandChannel <- CommandNext
}

func (p *Player) State() string {
	p.internals.stateMu.RLock()
	defer p.internals.stateMu.RUnlock()

	switch p.internals.state {
	case StateIdle:
		return "idle"

	case StatePlaying:
		return "playing"

	case StateResting:
		return "resting"

	case StateDead:
		return "dead"

	default:
		return "invalid"
	}
}

//////////////////////////////////
// State machine

type AudioPlayer interface {
	Play(io.ReadCloser) (time.Time, error)
	Stop()
	Close()
}

type PlayerState int32

const (
	StateIdle PlayerState = iota
	StatePlaying
	StateResting // Waiting in between songs
	StateDead
)

// Dependencies

type GPIO interface {
	Execute([]bool)
	SetAll(bool)
}

type PlayerStorage interface {
	ReadShowData(name string) (ProjectData, error)
	ReadAudio(name string) (afero.File, error)
	ListShows() ([]string, error)
}

type playerInternals struct {
	// Dependencies
	storage PlayerStorage
	config  *Config
	audio   AudioPlayer
	slaves  *Slaves
	gpio    GPIO

	runOnce sync.Once

	stateMu       sync.RWMutex
	state         PlayerState
	startTime     time.Time
	queue         CircularList[string]
	keyframes     []FlatKeyframe
	keyframeIndex int
}

func newPlayerInternals(config *Config, storage PlayerStorage, audio AudioPlayer, gpio GPIO) (*playerInternals, error) {
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
		gpio:    gpio,

		state: StateIdle,
	}, nil
}

func (pi *playerInternals) run(ctx context.Context, ch chan PlayerMessage) {
	pi.runOnce.Do(func() {
		defer func() {
			pi.stateMu.Lock()
			defer pi.stateMu.Unlock()

			pi.clearCurrentShow()
			pi.clearQueue()
			pi.state = StateDead
			pi.audio.Stop()
			pi.audio.Close()
		}()

		for {
			frameStart := time.Now()

			if err := ctx.Err(); err != nil {
				log.Warn().Err(err).Msg("Aborting player due to context error")
				return
			}

			if err := pi.loopIteration(ch); err != nil {
				log.Err(err).Msg("Player loop encountered an error")
				pi.enterIdle(false)
			}

			fps := pi.config.FramesPerSecond()
			nextFrameTime := frameStart.Add(time.Second / time.Duration(fps))
			delay := time.Until(nextFrameTime)

			if delay <= 0 {
				log.Warn().
					Dur("frame_duration", time.Since(frameStart)).
					Dur("desired_interval", time.Second/time.Duration(fps)).
					Msg("Framerate drop")
			} else {
				time.Sleep(delay)
			}
		}
	})
}

func (pi *playerInternals) loopIteration(ch chan PlayerMessage) error {
	// Locking the mutex on every loop iteration is very lazy and slow, but it's
	// easy. Ideally we would lock only when reading or writing the state fields.
	pi.stateMu.Lock()
	defer pi.stateMu.Unlock()

	// Handle incoming message
	select {
	case msg := <-ch:
		log.Info().Stringer("command", msg).Msg("Received message")

		switch msg := msg.(type) {
		case commandPlay:
			pi.clearCurrentShow()
			pi.clearQueue()
			if err := pi.playShow(msg.id, msg.startedAt); err != nil {
				return fmt.Errorf("playShow: %w", err)
			}

		case commandStatic:
			pi.enterIdle(bool(msg))

		case simpleCommand:
			switch msg {
			case CommandPlayAll:
				pi.clearCurrentShow()
				pi.clearQueue()
				if err := pi.playAllShows(); err != nil {
					return fmt.Errorf("playAllShows: %w", err)
				}

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
			log.Print("End of current show keyframes reached")
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
		// Wait for an additional 1 second before ending this song
		last := pi.keyframes[len(pi.keyframes)-1]
		elapsedBuffer := secs - last.Time
		extraBuffer := 1.0

		return elapsedBuffer >= extraBuffer, nil
	}

	next := pi.keyframes[pi.keyframeIndex]
	if next.Time <= secs {
		pi.gpio.Execute(next.States[pi.config.ChannelOffset():])
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

func (pi *playerInternals) enterIdle(state bool) {
	pi.clearCurrentShow()
	pi.clearQueue()

	pi.state = StateIdle

	pi.gpio.SetAll(state)

	pi.slaves.Static(state)
}

func (pi *playerInternals) playShow(id string, startedAt time.Time) error {
	data, err := pi.storage.ReadShowData(id)
	if err != nil {
		return err
	}
	pi.keyframes = data.FlatKeyframes()
	log.Debug().Int("keyframe_count", len(pi.keyframes)).Msg("Loaded keyframes")
	if len(pi.keyframes) == 0 {
		return fmt.Errorf("show %q had zero keyframes", id)
	}

	offset := pi.config.ChannelOffset()

	if offset >= len(pi.keyframes[0].States) {
		log.Warn().
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

	log.Info().
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
		pi.enterIdle(true)
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
		log.Info().
			Str("period", pi.config.RestPeriod().String()).
			Str("next_up", pi.queue.PeekNext()).
			Msg("Resting")

		pi.state = StateResting
		pi.startTime = time.Now()
	} else {
		// No more items in queue, stop
		pi.enterIdle(true)
	}
}

type Slaves struct {
	hosts []url.URL
}

func (s *Slaves) send(u string) {
	if _, err := http.Get(u); err != nil {
		log.Err(err).Msg("Failed to notify slave")
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

func (s *Slaves) Static(state bool) {
	for _, u := range s.hosts {
		go func() {
			u.Path += "/api/static"

			if state {
				q := u.Query()
				q.Add("value", "1")
				u.RawQuery = q.Encode()
			}

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
