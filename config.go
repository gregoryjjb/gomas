package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

type DurationMarshallable time.Duration

func (d DurationMarshallable) MarshalJSON() ([]byte, error) {
	return json.Marshal(time.Duration(d).String())
}

func (d *DurationMarshallable) UnmarshalJSON(b []byte) error {
	var v interface{}
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	switch value := v.(type) {
	case float64:
		*d = DurationMarshallable(time.Duration(value))
		return nil
	case string:
		tmp, err := time.ParseDuration(value)
		if err != nil {
			return err
		}
		*d = DurationMarshallable(tmp)
		return nil
	default:
		return errors.New("invalid duration")
	}
}

type GomasConfig struct {
	Pinout     []int                 `json:"pinout"`
	Bias       *DurationMarshallable `json:"bias"`
	RestPeriod *DurationMarshallable `json:"rest_period"`
}

func GetEnvOr(key string, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		value = fallback
	}
	return value
}

var Port = GetEnvOr("PORT", "1225")
var Host = GetEnvOr("HOST", "")

var NoEmbed = os.Getenv("GOMAS_NO_EMBED") != ""

// How many times per second to update the state of the lights
var FramesPerSecond float64 = 120

var DataDir string

func SetDataDir() {
	provided := os.Getenv("GOMAS_DATA_DIR")
	if provided == "" {
		DataDir, _ = filepath.Abs("./data")
		log.Info().Str("path", DataDir).Msg("Using default data directory")
	} else {
		DataDir, _ = filepath.Abs(provided)
		log.Info().Str("path", DataDir).Msg("Using provded data directory")
	}
}

const ConfigFilename = "config.json"

var (
	config      GomasConfig
	configMutex sync.RWMutex
)

func GetConfig() GomasConfig {
	configMutex.RLock()
	c := config
	configMutex.RUnlock()
	return c
}

func setConfig(c GomasConfig) {
	configMutex.Lock()
	config = c
	configMutex.Unlock()
}

func InitConfig() error {
	SetDataDir()

	// Data directory non-existence is a fatal error
	dirExists, err := FileExists(DataDir)
	if err != nil {
		return err
	}
	if !dirExists {
		return errors.New("data directory does not exist")
	}

	// Config file non-existence is only a log warning
	configPath := filepath.Join(DataDir, ConfigFilename)
	configExists, err := FileExists(configPath)
	if err != nil {
		return err
	}
	if !configExists {
		log.Warn().Str("path", configPath).Msg("Config file not found, using defaults")
	} else {
		configFile, err := os.Open(configPath)
		if err != nil {
			return err
		}
		defer configFile.Close()

		var c GomasConfig

		if err := json.NewDecoder(configFile).Decode(&c); err != nil {
			return err
		}

		remarshalled, err := json.Marshal(c)
		if err != nil {
			log.Panic().Err(err).Msg("Could not re-marshal config object (this should be impossible)")
		}

		setConfig(c)

		log.Info().
			Str("path", configPath).
			RawJSON("config", remarshalled).
			Msg("Loaded config file")
	}

	return nil
}
