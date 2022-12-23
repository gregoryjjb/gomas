package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5/middleware"
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

// Copied from https://github.com/ironstar-io/chizerolog
func LoggerMiddleware(logger *zerolog.Logger) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		fn := func(w http.ResponseWriter, r *http.Request) {
			log := logger.With().Logger()

			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

			t1 := time.Now()
			defer func() {
				t2 := time.Now()

				// Recover and record stack traces in case of a panic
				if rec := recover(); rec != nil {
					log.Error().
						Interface("recover_info", rec).
						Bytes("debug_stack", debug.Stack()).
						Msg("HTTP endpoint panic")

					http.Error(ww, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
				}

				// log end request
				log.Info().
					Str("type", "access").
					Timestamp().
					Fields(map[string]interface{}{
						"remote_ip":  r.RemoteAddr,
						"url":        r.URL.Path,
						"proto":      r.Proto,
						"method":     r.Method,
						"user_agent": r.Header.Get("User-Agent"),
						"status":     ww.Status(),
						"latency_ms": float64(t2.Sub(t1).Nanoseconds()) / 1000000.0,
						"bytes_in":   r.Header.Get("Content-Length"),
						"bytes_out":  ww.BytesWritten(),
					}).
					Msg("HTTP request")
			}()

			next.ServeHTTP(ww, r)
		}
		return http.HandlerFunc(fn)
	}
}
