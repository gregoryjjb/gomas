package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/pelletier/go-toml/v2"
	"github.com/rs/zerolog/log"
	"github.com/spf13/afero"
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
	Slaves          []string
}

func GetEnvOr(key string, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		value = fallback
	}
	return value
}

// fallback returns the first non-zero value
func fallback[T comparable](values ...T) T {
	var t T
	for _, value := range values {
		if value != t {
			return value
		}
	}
	return t
}

const ConfigFilename = "gomas.toml"

// FindToml returns the path to the config file that should be used
func FindToml(fs GomasFS, flags Flags, getEnv GetEnver) (string, error) {
	if flags.Config != "" {
		path, err := fs.Abs(flags.Config)
		if err != nil {
			return "", err
		}
		log.Debug().Str("path", path).Msg("Using config provided by --config flag")
		exists, err := afero.Exists(fs, path)
		if err != nil {
			return "", err
		}
		if !exists {
			return "", fmt.Errorf("config file provided by --config flag does not exist: %s", path)
		}
		return path, nil
	}

	if path := getEnv("GOMAS_CONFIG"); path != "" {
		path, err := fs.Abs(path)
		if err != nil {
			return "", err
		}
		log.Debug().Str("path", path).Msg("Using config file provided by environment")
		exists, err := afero.Exists(fs, path)
		if err != nil {
			return "", err
		}
		if !exists {
			return "", fmt.Errorf("config file provided by environment does not exist: %s", path)
		}
		return path, nil
	}

	var toSearch []string

	wd, err := fs.Abs("./")
	if err != nil {
		return "", err
	}
	toSearch = append(toSearch, wd)

	home, err := fs.HomeDir()
	if err != nil {
		return "", err
	}
	toSearch = append(toSearch, home)

	for _, p := range []string{wd, home} {
		path := filepath.Join(p, ConfigFilename)
		exists, err := afero.Exists(fs, path)
		if err != nil {
			return "", err
		}
		if exists {
			return path, nil
		}
	}

	return "", fmt.Errorf("could not find %s at any of the expected locations: %s", ConfigFilename, strings.Join(toSearch, ", "))
}

func ParseToml(fs GomasFS, path string) (*ConfigTOML, error) {
	configFile, err := fs.Open(path)
	if err != nil {
		return nil, err
	}
	defer configFile.Close()

	var c ConfigTOML
	if err := toml.NewDecoder(configFile).Decode(&c); err != nil {
		return nil, err
	}

	if c.DataDir == "" {
		return nil, fmt.Errorf("missing required config field data_dir")
	}
	dirExists, err := afero.DirExists(fs, c.DataDir)
	if err != nil {
		return nil, err
	}
	if !dirExists {
		return nil, fmt.Errorf("data directory not found: %q", c.DataDir)
	}

	return &c, nil
}

func LoadConfig(fs GomasFS, flags Flags, getEnv GetEnver) (*Config, error) {
	path, err := FindToml(fs, flags, getEnv)
	if err != nil {
		return nil, err
	}

	toml, err := ParseToml(fs, path)
	if err != nil {
		return nil, err
	}
	log.Info().Str("path", path).Interface("config", toml).Msg("Config TOML loaded")

	return &Config{
		toml:         toml,
		TomlPath:     path,
		Host:         getEnv("HOST"),
		Port:         fallback(getEnv("PORT"), "1225"),
		DisableEmbed: getEnv("GOMAS_DISABLE_EMBED") != "",
	}, nil
}

type Config struct {
	sync.RWMutex

	toml *ConfigTOML

	TomlPath     string
	Host         string
	Port         string
	DisableEmbed bool
}

func NewConfig(toml ConfigTOML) *Config {
	return &Config{toml: &toml}
}

func (c *Config) DataDir() string {
	c.RLock()
	defer c.RUnlock()

	return c.toml.DataDir
}

func (c *Config) Pinout() []int {
	c.RLock()
	defer c.RUnlock()

	cloned := make([]int, len(c.toml.Pinout))
	copy(cloned, c.toml.Pinout)
	return cloned
}

func (c *Config) Bias() time.Duration {
	c.RLock()
	defer c.RUnlock()

	if c.toml.Bias != nil {
		return time.Duration(*c.toml.Bias)
	}
	return time.Millisecond * 100
}

func (c *Config) RestPeriod() time.Duration {
	c.RLock()
	defer c.RUnlock()

	if c.toml.RestPeriod != nil {
		return time.Duration(*c.toml.RestPeriod)
	}
	return time.Second * 5
}

func (c *Config) FramesPerSecond() int {
	c.RLock()
	defer c.RUnlock()

	if c.toml.FramesPerSecond != nil {
		return *c.toml.FramesPerSecond
	}
	return 120
}

func (c *Config) SpeakerBuffer() time.Duration {
	c.RLock()
	defer c.RUnlock()

	if c.toml.SpeakerBuffer != nil {
		return time.Duration(*c.toml.SpeakerBuffer)
	}
	return time.Millisecond * 100
}
