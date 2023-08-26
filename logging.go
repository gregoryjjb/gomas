package main

import (
	"fmt"
	"gregoryjjb/gomas/circularbuffer"
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

// Colorization

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

// Should probably hook this up to something
var disableColor = false

func colorize(s interface{}, color int) string {
	if disableColor {
		return fmt.Sprintf("%s", s)
	}
	return fmt.Sprintf("\x1b[%dm%v\x1b[0m", color, s)
}

// BlockingWriter synchronzes all writes using a mutex
type BlockingWriter struct {
	w  io.Writer
	mu *sync.Mutex
}

func NewBlockingWriter(w io.Writer) BlockingWriter {
	return BlockingWriter{
		w:  w,
		mu: new(sync.Mutex),
	}
}

func (bw BlockingWriter) Write(p []byte) (int, error) {
	bw.mu.Lock()
	defer bw.mu.Unlock()

	return bw.w.Write(p)
}

// Console writer

func newConsoleWriter(out io.Writer) io.Writer {
	cw := zerolog.ConsoleWriter{
		Out:        out, //NewBlockingWriter(colorable.NewColorable(os.Stdout)),
		TimeFormat: time.RFC3339,
	}

	cw.FormatLevel = func(i interface{}) string {
		var l string
		if ll, ok := i.(string); ok {
			switch ll {
			case zerolog.LevelTraceValue:
				l = colorize("TRACE", colorMagenta)
			case zerolog.LevelDebugValue:
				l = colorize("DEBUG", colorYellow)
			case zerolog.LevelInfoValue:
				l = colorize("INFO ", colorGreen)
			case zerolog.LevelWarnValue:
				l = colorize("WARN ", colorRed)
			case zerolog.LevelErrorValue:
				l = colorize(colorize("ERROR", colorRed), colorBold)
			case zerolog.LevelFatalValue:
				l = colorize(colorize("FATAL", colorRed), colorBold)
			case zerolog.LevelPanicValue:
				l = colorize(colorize("PANIC", colorRed), colorBold)
			default:
				l = colorize(ll, colorBold)
			}
		} else {
			if i == nil {
				l = colorize("???  ", colorBold)
			} else {
				l = strings.ToUpper(fmt.Sprintf("%-5s", i))[0:5]
			}
		}

		return fmt.Sprintf("| %s |", l)
	}

	return cw
}

// BufferedWriter saves everything written to it to a buffer
type BufferedWriter struct {
	buffer *circularbuffer.CircularBuffer[string]
	w      io.Writer
}

func (bw BufferedWriter) Write(p []byte) (int, error) {
	defer bw.buffer.Push(string(p))
	return bw.w.Write(p)
}

// Initialization

var bufferedLogger BufferedWriter

func BufferedLogs(w io.Writer) {
	bufferedLogger.buffer.Each(func(row string) {
		w.Write([]byte(row))
	})
}

func BufferedLogsArray() []string {
	var logs []string
	bufferedLogger.buffer.Each(func(row string) {
		logs = append(logs, row)
	})
	return logs
}

func InitializeLogger() {
	bufferedLogger = BufferedWriter{
		buffer: circularbuffer.New[string](100),
		w:      NewBlockingWriter(colorable.NewColorable(os.Stdout)),
	}

	console := newConsoleWriter(bufferedLogger)

	log.Logger = log.Output(console)
}

// Middleware

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
