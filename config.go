package main

import (
	"cmp"
	"fmt"
	"path/filepath"
	"strings"
	"sync/atomic"
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
	ChannelOffset   int                   `toml:"channel_offset"`
	Bias            *DurationMarshallable `toml:"bias"`
	RestPeriod      *DurationMarshallable `toml:"rest_period"`
	FramesPerSecond *int                  `toml:"frames_per_second"`
	SpeakerBuffer   *DurationMarshallable `toml:"speaker_buffer"`
	Slaves          []string              `toml:"slaves"`
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
		return nil, fmt.Errorf("open config: %w", err)
	}
	defer configFile.Close()

	var c ConfigTOML
	if err := toml.NewDecoder(configFile).Decode(&c); err != nil {
		return nil, fmt.Errorf("decode config: %w", err)
	}

	if c.DataDir == "" {
		return nil, fmt.Errorf("missing required config field data_dir")
	}

	dirExists, err := afero.DirExists(fs, c.DataDir)
	if err != nil {
		return nil, fmt.Errorf("checking for data directory existence: %w", err)
	}

	if !dirExists {
		return nil, fmt.Errorf("data directory not found: %q", c.DataDir)
	}

	if c.ChannelOffset < 0 {
		return nil, fmt.Errorf("channel_offset must not be negative, received: %q", c.ChannelOffset)
	}

	return &c, nil
}

type Config struct {
	fs     GomasFS
	getEnv GetEnver
	flags  Flags

	toml atomic.Pointer[ConfigTOML]
}

func NewConfig(fs GomasFS, flags Flags, getEnv GetEnver) (*Config, error) {
	config := &Config{
		fs:     fs,
		flags:  flags,
		getEnv: getEnv,
	}

	// In theory this can be called again at any point to reload config without restarting
	if err := config.loadToml(); err != nil {
		return nil, fmt.Errorf("load toml: %w", err)
	}

	return config, nil
}

func (c *Config) loadToml() error {
	path, err := FindToml(c.fs, c.flags, c.getEnv)
	if err != nil {
		return fmt.Errorf("find toml: %w", err)
	}

	toml, err := ParseToml(c.fs, path)
	if err != nil {
		return fmt.Errorf("parse toml: %w", err)
	}

	c.toml.Store(toml)

	log.Info().Str("path", path).Interface("config", toml).Msg("Loaded TOML config")

	return nil
}

func (c *Config) Host() string {
	return c.getEnv("HOST")
}

func (c *Config) Port() string {
	return cmp.Or(c.getEnv("PORT"), "1225")
}

func (c *Config) DisableEmbed() bool {
	return c.getEnv("GOMAS_DISABLE_EMBED") != ""
}

func (c *Config) DataDir() string {
	return c.toml.Load().DataDir
}

func (c *Config) Pinout() []int {
	return c.toml.Load().Pinout
}

func (c *Config) ChannelOffset() int {
	return c.toml.Load().ChannelOffset
}

func (c *Config) Bias() time.Duration {
	bias := c.toml.Load().Bias

	if bias != nil {
		return time.Duration(*bias)
	}

	return time.Millisecond * 100
}

func (c *Config) RestPeriod() time.Duration {
	restPeriod := c.toml.Load().RestPeriod

	if restPeriod != nil {
		return time.Duration(*restPeriod)
	}

	return time.Second * 5
}

func (c *Config) FramesPerSecond() int {
	return fallbackIfNil(c.toml.Load().FramesPerSecond, 120)
}

func (c *Config) SpeakerBuffer() time.Duration {
	speakerBuffer := c.toml.Load().SpeakerBuffer

	if speakerBuffer != nil {
		return time.Duration(*speakerBuffer)
	}

	return time.Duration(time.Millisecond * 100)
}

func (c *Config) Slaves() []string {
	return c.toml.Load().Slaves
}

func fallbackIfNil[T any](a *T, b T) T {
	if a == nil {
		return b
	}

	return *a
}
