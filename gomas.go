package main

import (
	"cmp"
	"context"
	"flag"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/rs/zerolog/log"

	"gregoryjjb/gomas/gpio"
)

type GetEnver func(string) string

func init() {
	InitializeLogger()
}

// Populated by ldflags (ugh)
var (
	version            string
	buildUnixTimestamp string
	commitHash         string
)

type BuildInfo struct {
	Version    string
	Time       time.Time
	CommitHash string
}

func getBuildInfo() BuildInfo {
	ts, _ := strconv.ParseInt(buildUnixTimestamp, 10, 64)
	buildTime := time.Unix(ts, 0)

	return BuildInfo{
		Version:    cmp.Or(version, "[dev]"),
		Time:       buildTime,
		CommitHash: commitHash,
	}
}

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

	buildInfo := getBuildInfo()

	flags, err := parseFlags(args)
	if err != nil {
		return err
	}

	if flags.Version {
		fmt.Println("Gomas version:", buildInfo.Version)
		fmt.Println("Built on:", buildInfo.Time)
		fmt.Println("Commit hash:", buildInfo.CommitHash)
		return nil
	}

	if flags.Systemd {
		return SystemdServiceFile(flags)
	}

	log.Info().
		Str("version", buildInfo.Version).
		Time("build_time", buildInfo.Time).
		Str("commit_hash", buildInfo.CommitHash).
		Msg("Initializing Gomas")

	config, err := NewConfig(fs, flags, getEnv)
	if err != nil {
		return err
	}

	storage := NewStorage(fs, config)

	if flags.Migrate {
		log.Info().Msg("Migrating show directory structure")
		return storage.Migrate()
	}

	// Initialize GPIO
	if err := gpio.Init(config.Pinout()); err != nil {
		return err
	}

	audio, err := NewSpeakerPlayer(config.SpeakerBuffer())
	if err != nil {
		return err
	}

	player := NewPlayer(ctx, config, storage, audio, concreteGPIO{})

	return StartServer(config, buildInfo, player, storage)
}

type Flags struct {
	Version bool
	Systemd bool
	Config  string
	User    string
	Migrate bool
}

func parseFlags(args []string) (Flags, error) {
	var f Flags
	set := flag.NewFlagSet("", flag.ContinueOnError)

	set.BoolVar(&f.Version, "version", false, "Print version")
	set.BoolVar(&f.Systemd, "systemd", false, "Generate and print systemd service file")
	set.StringVar(&f.Config, "config", "", "Path to config file")
	set.StringVar(&f.User, "user", "pi", "User to run as for systemd")
	set.BoolVar(&f.Migrate, "migrate", false, "Migrate from old to new dir structure")

	if err := set.Parse(args); err != nil {
		return Flags{}, err
	}

	return f, nil
}

type concreteGPIO struct {}

func (c concreteGPIO) Execute(states []bool) {
	gpio.Execute(states)
}

func (c concreteGPIO) SetAll(state bool) {
	gpio.SetAll(state)
}
