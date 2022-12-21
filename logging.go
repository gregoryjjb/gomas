package main

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/mattn/go-colorable"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

const (
	colorBlack = iota + 30
	colorRed
	colorGreen
	colorYellow
	colorBlue
	colorMagenta
	colorCyan
	colorWhite

	colorBold     = 1
	colorDarkGray = 90
)

func colorize(s interface{}, c int, disabled bool) string {
	if disabled {
		return fmt.Sprintf("%s", s)
	}
	return fmt.Sprintf("\x1b[%dm%v\x1b[0m", c, s)
}

type ThreadSafeWriter struct {
	w io.Writer
}

var globalStdoutMutex sync.RWMutex

// This is blocking but eh good enough to avoid overlapping logs
func (tsw ThreadSafeWriter) Write(p []byte) (int, error) {
	globalStdoutMutex.Lock()
	n, err := tsw.w.Write(p)
	globalStdoutMutex.Unlock()
	return n, err
}

func NewThreadSafeWriter(w io.Writer) ThreadSafeWriter {
	return ThreadSafeWriter{w: w}
}

func InitializeLogger() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	output := zerolog.ConsoleWriter{
		Out:        NewThreadSafeWriter(colorable.NewColorable(os.Stdout)),
		TimeFormat: time.RFC3339,
	}

	noColor := false

	output.FormatLevel = func(i interface{}) string {
		var l string
		if ll, ok := i.(string); ok {
			switch ll {
			case zerolog.LevelTraceValue:
				l = colorize("TRACE", colorMagenta, noColor)
			case zerolog.LevelDebugValue:
				l = colorize("DEBUG", colorYellow, noColor)
			case zerolog.LevelInfoValue:
				l = colorize("INFO ", colorGreen, noColor)
			case zerolog.LevelWarnValue:
				l = colorize("WARN ", colorRed, noColor)
			case zerolog.LevelErrorValue:
				l = colorize(colorize("ERROR", colorRed, noColor), colorBold, noColor)
			case zerolog.LevelFatalValue:
				l = colorize(colorize("FATAL", colorRed, noColor), colorBold, noColor)
			case zerolog.LevelPanicValue:
				l = colorize(colorize("PANIC", colorRed, noColor), colorBold, noColor)
			default:
				l = colorize(ll, colorBold, noColor)
			}
		} else {
			if i == nil {
				l = colorize("???  ", colorBold, noColor)
			} else {
				l = strings.ToUpper(fmt.Sprintf("%-5s", i))[0:5]
			}
		}

		return fmt.Sprintf("| %s |", l)
	}

	log.Logger = log.Output(output)
}
