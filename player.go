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
	Execute(map[string]bool)
	SetAll(bool)
}

type PlayerStorage interface {
	ReadShowData(name string) (ShowData, error)
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

	stateMu   sync.RWMutex
	state     PlayerState
	startTime time.Time
	queue     CircularList[string]

	keyframes        map[string][]ShowKeyframe
	keyframeIndex    map[string]int
	lastKeyframeTime float64

	// This is only a field here instead of a local variable
	// so we can reuse the same memory every frame
	gpioFrame map[string]bool
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

		fps := pi.config.FramesPerSecond()
		ticker := time.NewTicker(time.Second / time.Duration(fps))
		defer ticker.Stop()

		for {
			select {
			// Context cancelled
			case <-ctx.Done():
				log.Info().Err(ctx.Err()).Msg("Player aborting due to context")
				return

			// Received a command message
			case msg := <-ch:
				if err := pi.handleMessage(msg); err != nil {
					log.Err(err).Stringer("command", msg).Msg("Error handling message")
				}

			// Time for a loop iteration
			case <-ticker.C:
				if err := pi.loopIteration(); err != nil {
					log.Err(err).Msg("Player loop encountered an error")
				}
			}
		}
	})
}

func (pi *playerInternals) handleMessage(msg PlayerMessage) error {
	// Locking the mutex on every loop iteration is very lazy and slow, but it's
	// easy. Ideally we would lock only when reading or writing the state fields.
	pi.stateMu.Lock()
	defer pi.stateMu.Unlock()

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

	return nil
}

func (pi *playerInternals) loopIteration() error {
	// Locking the mutex on every loop iteration is very lazy and slow, but it's
	// easy. Ideally we would lock only when reading or writing the state fields.
	pi.stateMu.Lock()
	defer pi.stateMu.Unlock()

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

	clear(pi.gpioFrame)

	for trackID, keyframes := range pi.keyframes {
		index := pi.keyframeIndex[trackID]

		if index >= len(keyframes) {
			continue
		}

		next := keyframes[index]
		if next.Timestamp <= secs {
			pi.gpioFrame[trackID] = next.Value > 0
			pi.keyframeIndex[trackID] += 1
		}
	}

	if len(pi.gpioFrame) > 0 {
		pi.gpio.Execute(pi.gpioFrame)
	}

	// 1 second extra buffer before ending song
	return secs > pi.lastKeyframeTime+1.0, nil
}

func (pi *playerInternals) clearCurrentShow() {
	pi.audio.Stop()
	pi.keyframes = nil
	pi.lastKeyframeTime = 0

	for k := range pi.keyframeIndex {
		pi.keyframeIndex[k] = 0
	}
}

func (pi *playerInternals) clearQueue() {
	pi.queue.Clear()
}

func (pi *playerInternals) enterIdle(state bool) {
	log.Debug().Bool("idle_state", state).Msg("Entering idle")

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

	pinout := pi.config.Pinout()

	flat := make(map[string][]ShowKeyframe)
	pi.lastKeyframeTime = 0

	var availableTrackIDs []string

	var visit func(ShowNodes)
	visit = func(nodes ShowNodes) {
		for _, node := range nodes {
			switch n := node.(type) {
			case *ShowGroup:
				visit(n.Children)
			case *ShowTrack:
				if n.ID == BeatTrackID {
					continue
				}
				availableTrackIDs = append(availableTrackIDs, n.ID)

				if _, ok := pinout[n.ID]; ok {
					// Track has a pinout on this device! Keep it
					flat[n.ID] = n.Keyframes
				}

				// Include all lighting tracks in the duration, even unmapped ones.
				if len(n.Keyframes) > 0 {
					lastKeyframe := n.Keyframes[len(n.Keyframes)-1]

					if lastKeyframe.Timestamp > pi.lastKeyframeTime {
						pi.lastKeyframeTime = lastKeyframe.Timestamp
					}
				}
			}
		}
	}
	visit(data.Tracks)

	pi.keyframes = flat
	pi.keyframeIndex = make(map[string]int, len(pi.keyframes))
	for track := range pi.keyframes {
		pi.keyframeIndex[track] = 0
	}

	pi.gpioFrame = make(map[string]bool, len(pinout))

	log.Debug().Int("keyframe_count", len(pi.keyframes)).Msg("Loaded keyframes")
	if len(pi.keyframes) == 0 {

		log.Warn().
			Str("show_id", id).
			Strs("available_tracks", availableTrackIDs).
			Msg("No available tracks matched the configured pinout; no GPIO events will play")
	}

	pi.gpio.SetAll(false)

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

		// Since the slaves are playing only a single song,
		// they will enter idle and set the pins on that way
		pi.gpio.SetAll(true)

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
