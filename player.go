package main

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/faiface/beep"
	"github.com/faiface/beep/mp3"
	"github.com/faiface/beep/speaker"
)

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
	go workerLoop(ch, state)

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

	fmt.Printf("loaded %d keyframes\n", len(kfs))

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
		var strs []string
		for _, n := range current.States {
			if n {
				strs = append(strs, "#")
			} else {
				strs = append(strs, "_")
			}
		}
		fmt.Println(strings.Join(strs, ""))
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

	// Buffer?
	// buffer := beep.NewBuffer(format)
	// buffer.Append(streamer)
	// streamer.Close()
	// s2 := buffer.Streamer(0, buffer.Len())
	// ap.streamer = s2

	ap.streamer = streamer

	done := make(chan bool)
	speaker.Play(beep.Seq(ap.streamer, beep.Callback(func() {
		done <- true
	})))

	return time.Now(), nil
}

// func (ap *AudioPlayer) CurrentTime() time.Duration {
// 	speaker.Lock()
// 	pos := ap.format.SampleRate.D(ap.streamer.Position())
// 	speaker.Unlock()
// 	return pos
// }

//////////////////////////////////
// State machine

type PlayerState string

const (
	StateIdle    = "idle"
	StatePlaying = "playing"
	StateResting = "resting" // Waiting in between songs
)

const restPeriod = time.Second * 3

func workerLoop(channel chan PlayerMessage, externalState *ExternalPlayerState) {
	var state PlayerState = StateIdle
	var startTime time.Time

	var queue []string
	queueIndex := 0

	g, err := NewGPIO()
	if err != nil {
		fmt.Printf("FATAL GPIO ERROR: %e\n", err)
	}

	audioPlayer := NewAudioPlayer()
	keyframePlayer := &KeyframePlayer{gpio: g}

	// Predeclare
	// var err error
	var enterIdle func()

	handleError := func(format string, a ...any) {
		fmt.Printf(format, a...)
		enterIdle()
	}

	loadShow := func(id string) {
		state = StatePlaying
		fmt.Printf("loading show: %s\n", id)
		externalState.Set(fmt.Sprintf("Now playing %s", id))

		err := keyframePlayer.Load(id)
		if err != nil {
			handleError("FATAL ERROR when loading keyframes: %e\n", err)
			return
		}

		audioPath, err := ShowAudioPath(id)
		if err != nil {
			handleError("FATAL ERROR when getting audio path: %e\n", err)
			return
		}

		startTime, err = audioPlayer.Play(audioPath)
		if err != nil {
			handleError("FATAL ERROR when playing audio: %e\n", err)
			return
		}
	}

	playAllShows := func() {
		shows, err := ListShows()
		if err != nil {
			handleError("failed to play all: %e\n", err)
			return
		}

		var ids []string
		for _, show := range shows {
			if show.HasAudio {
				ids = append(ids, show.ID)
			}
		}

		if len(ids) == 0 {
			handleError("cannot play all, no playable shows found")
			return
		}

		queue = ids
		queueIndex = 0
		loadShow(queue[0])
	}

	clearCurrentShow := func() {
		audioPlayer.Stop()
		keyframePlayer.Unload()
	}

	clearQueue := func() {
		queue = nil
		queueIndex = 0
	}

	nextQueueIndex := func() int {
		nqi := queueIndex + 1
		if nqi >= len(queue) {
			nqi = 0
		}
		return nqi
	}

	nextUp := func() string {
		if len(queue) == 0 {
			return ""
		}
		return queue[nextQueueIndex()]
	}

	advanceQueue := func() {
		queueIndex = nextQueueIndex()
	}

	loadNextShow := func() {
		clearCurrentShow()
		if len(queue) > 0 {
			advanceQueue()
			loadShow(queue[queueIndex])
		}
	}

	handleShowEnd := func() {
		clearCurrentShow()

		if len(queue) > 1 {
			fmt.Printf("resting for %s, next up: %s\n", restPeriod, nextUp())
			state = StateResting
			startTime = time.Now()
			externalState.Set(fmt.Sprintf("Next up: %s", nextUp()))

			// If there's more than one item in the queue we are doing a playlist
			// advanceQueue()
			// loadShow(queue[queueIndex])
		} else {
			// No more items in queue, stop
			enterIdle()
		}
	}

	enterIdle = func() {
		clearCurrentShow()
		clearQueue()
		state = StateIdle
	}

	for true {
		// fmt.Printf("Current state: %s\n", state)

		// Handle state change
		select {
		case msg := <-channel:
			fmt.Printf("player received message: %s\n", msg.Command)
			switch command := msg.Command; command {
			// Playing
			case CommandPlay:
				// One might be playing, stop it
				clearCurrentShow()
				clearQueue()

				state = StatePlaying
				queue = append(queue, msg.Value)

				loadShow(msg.Value)

			case CommandPlayAll:
				clearCurrentShow()
				clearQueue()

				state = StatePlaying
				playAllShows()

			// Stopping
			case CommandStop:
				enterIdle()

			case CommandNext:
				loadNextShow()

			case "kill":
				fmt.Println("Killing player thread")
				clearCurrentShow()
				return

			}
		default:
			// Do nothing
		}

		// Do unit of work
		// switch state {...}
		switch state {
		case StatePlaying:
			t := time.Since(startTime)
			// t := audioPlayer.CurrentTime()
			done, err := keyframePlayer.Execute(t)
			if err != nil {
				fmt.Printf("FATAL ERROR EXECUTING KEYFRAME: %e\n", err)
			}
			if done {
				fmt.Println("done signal received, ending current show")
				handleShowEnd()
			}

		case StateResting:
			t := time.Since(startTime)
			if t >= restPeriod {
				state = StatePlaying
				loadNextShow()
			}

			// fmt.Printf("At song time: %v\n", t)
		}

		time.Sleep(time.Millisecond)
	}
}
