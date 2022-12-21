package main

import (
	"flag"
	"fmt"

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

const GomasVersion = "69.420"

func main() {
	versionFlag := flag.Bool("version", false, "Print version")
	flag.Parse()

	if *versionFlag {
		fmt.Println("Gomas version ", GomasVersion)
		return
	}

	log.Info().Str("version", GomasVersion).Msg("Initializing Gomas")

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
