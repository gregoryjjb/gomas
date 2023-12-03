package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/pelletier/go-toml/v2"
	"github.com/rs/zerolog/log"
)

type DurationMarshallable time.Duration

func (d DurationMarshallable) MarshalText() ([]byte, error) {
	return []byte(time.Duration(d).String()), nil
}

func (d *DurationMarshallable) UnmarshalText(b []byte) error {
	parsed, err := time.ParseDuration(string(b))
	if err != nil {
		return err
	}
	*d = DurationMarshallable(parsed)
	return nil
}

type ConfigTOML struct {
	DataDir         string                `toml:"data_dir"`
	Pinout          []int                 `toml:"pinout"`
	Bias            *DurationMarshallable `toml:"bias"`
	RestPeriod      *DurationMarshallable `toml:"rest_period"`
	FramesPerSecond *int                  `toml:"frames_per_second"`
	SpeakerBuffer   *DurationMarshallable `toml:"speaker_buffer"`
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

const ConfigFilename = "gomas.toml"

var absDataDir string

var (
	config      ConfigTOML
	configMutex sync.RWMutex
)

func setConfig(c ConfigTOML) {
	configMutex.Lock()
	config = c
	configMutex.Unlock()
}

func FindConfigFile() (string, error) {
	configPath := ConfigFlag
	if configPath != "" {
		log.Debug().Str("path", configPath).Msg("Using config file provided by --config flag")
		exists, err := Exists(configPath)
		if err != nil {
			return "", err
		}
		if !exists {
			return "", fmt.Errorf("config file provided by --config flag does not exist: %q", configPath)
		}
		return configPath, nil
	}

	configPath = os.Getenv("GOMAS_CONFIG")
	if configPath != "" {
		log.Debug().Str("path", configPath).Msg("Using config file provided by environment")
		exists, err := Exists(configPath)
		if err != nil {
			return "", err
		}
		if !exists {
			return "", fmt.Errorf("config file provided by environment does not exist: %q", configPath)
		}
		return configPath, nil
	}

	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	for _, p := range []string{wd, home} {
		if configPath != "" {
			break
		}

		path := filepath.Join(p, ConfigFilename)
		log.Debug().Str("path", path).Msg("Searching for config file")
		exists, err := Exists(path)
		if err != nil {
			return "", err
		}
		if exists {
			configPath = path
		}
	}

	if configPath == "" {
		return "", fmt.Errorf("could not find config file anywhere")
	}

	return configPath, nil
}

func InitConfig() error {
	configPath, err := FindConfigFile()
	if err != nil {
		return err
	}

	log.Info().Str("path", configPath).Msg("Reading config file")

	configFile, err := os.Open(configPath)
	if err != nil {
		return err
	}
	defer configFile.Close()

	var c ConfigTOML
	if err := toml.NewDecoder(configFile).Decode(&c); err != nil {
		return err
	}

	log.Info().Interface("config", c).Msg("Config loaded")

	if c.DataDir == "" {
		return fmt.Errorf("missing required config field data_dir")
	}
	abs, err := filepath.Abs(c.DataDir)
	if err != nil {
		return err
	}
	dirExists, err := Exists(abs)
	if err != nil {
		return err
	}
	if !dirExists {
		return fmt.Errorf("data directory not found: %q", abs)
	}

	configMutex.Lock()
	absDataDir = abs
	config = c
	configMutex.Unlock()

	return nil
}

func GetDataDir() string {
	configMutex.RLock()
	defer configMutex.RUnlock()

	return absDataDir
}

func GetPinout() []int {
	configMutex.RLock()
	defer configMutex.RUnlock()

	cloned := make([]int, len(config.Pinout))
	copy(cloned, config.Pinout)
	return cloned
}

func GetBias() time.Duration {
	configMutex.RLock()
	defer configMutex.RUnlock()

	if config.Bias != nil {
		return time.Duration(*config.Bias)
	}
	return time.Millisecond * 100
}

func GetRestPeriod() time.Duration {
	configMutex.RLock()
	defer configMutex.RUnlock()

	if config.RestPeriod != nil {
		return time.Duration(*config.RestPeriod)
	}
	return time.Second * 5
}

func GetFramesPerSecond() int {
	configMutex.RLock()
	defer configMutex.RUnlock()

	if config.FramesPerSecond != nil {
		return *config.FramesPerSecond
	}
	return 120
}

func GetSpeakerBuffer() time.Duration {
	configMutex.RLock()
	defer configMutex.RUnlock()

	if config.SpeakerBuffer != nil {
		return time.Duration(*config.SpeakerBuffer)
	}
	return time.Millisecond * 100
}
