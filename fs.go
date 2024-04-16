package main

import (
	"os"
	"path/filepath"

	"github.com/spf13/afero"
)

// GomasFS is an Afero FS with added functionality
// to replicate OS filesystems in testing
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

type gomasMemFS struct {
	afero.Fs
}

func NewGomasMemFS() GomasFS {
	return &gomasMemFS{
		afero.NewMemMapFs(),
	}
}

func (g *gomasMemFS) Abs(path string) (string, error) {
	return path, nil
}

func (g *gomasMemFS) HomeDir() (string, error) {
	return "/", nil
}
