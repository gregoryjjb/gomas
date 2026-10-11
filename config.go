package main

import (
	"cmp"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
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

type Role string

const (
	RoleStandalone Role = "standalone"
	RoleMaster     Role = "master"
	RoleSlave      Role = "slave"
)

type ConfigTOML struct {
	raw string

	Role Role `toml:"role"`

	DataDir         string                `toml:"data_dir"`
	Pinout          map[string]int        `toml:"pinout"`
	Bias            *DurationMarshallable `toml:"bias"`
	RestPeriod      *DurationMarshallable `toml:"rest_period"`
	FramesPerSecond *int                  `toml:"frames_per_second"`
	SpeakerBuffer   *DurationMarshallable `toml:"speaker_buffer"`
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

func ParseToml(raw string, fs GomasFS) (*ConfigTOML, error) {
	var c ConfigTOML
	if err := toml.Unmarshal([]byte(raw), &c); err != nil {
		return nil, fmt.Errorf("decode config: %w", err)
	}

	c.raw = raw

	if c.Role == "" {
		c.Role = RoleStandalone
	}
	switch c.Role {
	case RoleStandalone, RoleMaster, RoleSlave:
	default:
		return nil, fmt.Errorf("invalid role %q: expected standalone, master, or slave", c.Role)
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

	pins := make(map[int]string)
	for name, pin := range c.Pinout {
		if strings.TrimSpace(name) == "" || name == BeatTrackID {
			return nil, fmt.Errorf("invalid GPIO channel name %q", name)
		}
		if pin < 0 || pin > 53 {
			return nil, fmt.Errorf("channel %q: BCM pin must be between 0 and 53", name)
		}
		if other, ok := pins[pin]; ok {
			return nil, fmt.Errorf("channels %q and %q share BCM pin %d", other, name, pin)
		}
		pins[pin] = name
	}

	return &c, nil
}

type Config struct {
	fs     GomasFS
	getEnv GetEnver
	flags  Flags

	tomlPath string
	toml     atomic.Pointer[ConfigTOML]
}

func NewConfig(fs GomasFS, flags Flags, getEnv GetEnver) (*Config, error) {

	path, err := FindToml(fs, flags, getEnv)
	if err != nil {
		return nil, fmt.Errorf("find config toml: %w", err)
	}

	config := &Config{
		fs:       fs,
		flags:    flags,
		getEnv:   getEnv,
		tomlPath: path,
	}

	// In theory this can be called again at any point to reload config without restarting
	if err := config.loadToml(); err != nil {
		return nil, fmt.Errorf("load toml: %w", err)
	}

	return config, nil
}

func (c *Config) loadToml() error {
	configFile, err := c.fs.Open(c.tomlPath)
	if err != nil {
		return fmt.Errorf("open config: %w", err)
	}
	defer configFile.Close()

	s, err := io.ReadAll(configFile)
	if err != nil {
		return fmt.Errorf("read config: %w", err)
	}

	toml, err := ParseToml(string(s), c.fs)
	if err != nil {
		return fmt.Errorf("parse toml: %w", err)
	}

	c.toml.Store(toml)

	log.Info().Str("path", c.tomlPath).Interface("config", toml).Msg("Loaded TOML config")

	return nil
}

func (c *Config) WriteToml(s string) error {
	toml, err := ParseToml(s, c.fs)
	if err != nil {
		return fmt.Errorf("parsing new toml: %w", err)
	}

	configFile, err := c.fs.OpenFile(c.tomlPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, os.ModePerm)
	if err != nil {
		return fmt.Errorf("open config: %w", err)
	}
	defer configFile.Close()

	if _, err := configFile.Write([]byte(s)); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	c.toml.Store(toml)

	log.Info().Str("path", c.tomlPath).Interface("config", toml).Msg("Updated TOML config")

	return nil
}

func (c *Config) RawToml() string {
	return c.toml.Load().raw
}

func (c *Config) ConfigPath() string {
	return c.tomlPath
}

func (c *Config) Host() string {
	return cmp.Or(c.getEnv("HOST"), "127.0.0.1")
}

func (c *Config) Role() Role {
	return c.toml.Load().Role
}

func (c *Config) Port() string {
	return cmp.Or(c.getEnv("PORT"), "1225")
}

func (c *Config) DataDir() string {
	return c.toml.Load().DataDir
}

func (c *Config) ChannelNames() []string {
	names := make([]string, 0, len(c.toml.Load().Pinout))
	for name := range c.toml.Load().Pinout {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

func (c *Config) Pinout() map[string]int {
	return c.toml.Load().Pinout
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
	return deref(c.toml.Load().FramesPerSecond, 120)
}

func (c *Config) SpeakerBuffer() time.Duration {
	speakerBuffer := c.toml.Load().SpeakerBuffer

	if speakerBuffer != nil {
		return time.Duration(*speakerBuffer)
	}

	return time.Duration(time.Millisecond * 100)
}

func deref[T any](p *T, fallback T) T {
	if p == nil {
		return fallback
	}

	return *p
}
