package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/spf13/afero"

	"gregoryjjb/gomas/gpio"
)

type GetEnver func(string) string

func PanicOnError(err error) {
	if err != nil {
		panic(err)
	}
}

func init() {
	InitializeLogger()
}

// Populated by ldflags (ugh)
var (
	version            string
	buildUnixTimestamp string
	commitHash         string
)

func main() {
	ctx := context.Background()
	if err := run(ctx, os.Args[1:], os.Getenv, newGomasOSFS()); err != nil {
		log.Err(err).Msg("Gomas crashed")
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, getEnv GetEnver, fs GomasFS) error {
	// // We can't gracefully shut down Oto because it has an infinite loop goroutine:
	// // https://github.com/ebitengine/oto/blob/457cd3ebf3f3fdb6605ca9f382bad90e6de49b54/driver_darwin.go#L167
	// ctx, cancel := signal.NotifyContext(ctx, os.Interrupt)
	// defer cancel()

	ts, _ := strconv.ParseInt(buildUnixTimestamp, 10, 64)
	buildTime := time.Unix(ts, 0)

	flags, err := parseFlags(args)
	if err != nil {
		return err
	}

	if flags.Version {
		fmt.Println("Gomas version:", version)
		fmt.Println("Built on:", buildTime)
		fmt.Println("Commit hash:", commitHash)
		return nil
	}

	if flags.Systemd {
		SystemdServiceFile()
		return nil
	}

	log.Info().
		Str("version", version).
		Str("build_timestamp", buildTime.Format(time.RFC3339)).
		Str("commit_hash", commitHash).
		Msg("Initializing Gomas")

	config, err := LoadConfig(fs, flags, getEnv)
	if err != nil {
		return err
	}

	// Initialize GPIO
	if err := gpio.Init(config.Pinout()); err != nil {
		return err
	}

	audio, err := NewSpeakerPlayer(config.SpeakerBuffer())
	if err != nil {
		return err
	}

	storage := NewStorage(fs, config)
	player := NewPlayer(ctx, config, storage, audio)

	return StartServer(config, player, storage)
}

type Flags struct {
	Version bool
	Systemd bool
	Config  string
}

func parseFlags(args []string) (Flags, error) {
	var f Flags
	set := flag.NewFlagSet("", flag.ContinueOnError)

	set.BoolVar(&f.Version, "version", false, "Print version")
	set.BoolVar(&f.Systemd, "systemd", false, "Generate and print systemd service file")
	set.StringVar(&f.Config, "config", "", "Path to config file")

	if err := set.Parse(args); err != nil {
		return Flags{}, err
	}

	return f, nil
}

type GomasFS interface {
	afero.Fs
	Abs(string) (string, error)
	HomeDir() (string, error)
}

type gomasOSFS struct {
	afero.Fs
}

func newGomasOSFS() GomasFS {
	return &gomasOSFS{
		afero.NewOsFs(),
	}
}

func (g *gomasOSFS) Abs(path string) (string, error) {
	return filepath.Abs(path)
}

func (g *gomasOSFS) HomeDir() (string, error) {
	return os.UserHomeDir()
}
