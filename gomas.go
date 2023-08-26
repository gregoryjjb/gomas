package main

import (
	"flag"
	"fmt"
	"strconv"
	"time"

	"github.com/rs/zerolog/log"

	"gregoryjjb/gomas/gpio"
)

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
	ts, _ := strconv.ParseInt(buildUnixTimestamp, 10, 64)
	buildTime := time.Unix(ts, 0)

	versionFlag := flag.Bool("version", false, "Print version")
	systemdFlag := flag.Bool("systemd", false, "Print systemd service file")
	flag.Parse()

	if *versionFlag {
		fmt.Println("Gomas version:", version)
		fmt.Println("Built on:", buildTime)
		fmt.Println("Commit hash:", commitHash)
		return
	}

	if *systemdFlag {
		SystemdServiceFile()
		return
	}

	log.Info().
		Str("version", version).
		Str("build_timestamp", buildTime.Format(time.RFC3339)).
		Str("commit_hash", commitHash).
		Msg("Initializing Gomas")

	// Initialize Config
	if err := InitConfig(); err != nil {
		log.Fatal().Err(err).Msg("Config initialization failed")
	}

	// Initialize GPIO
	if err := gpio.Init(GetConfig().Pinout); err != nil {
		log.Err(err).Msg("GPIO initialization failed")
	}

	player := NewPlayer()

	if err := StartServer(player); err != nil {
		log.Err(err).Msg("Server closed with error")
	}
}
