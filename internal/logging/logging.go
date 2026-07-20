package logging

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// LevelTrace is below Debug, for very chatty per-request tracing.
const LevelTrace = slog.Level(-8)

// Init sets the global slog logger and its minimum level.
func Init(debug, trace bool) {
	level := new(slog.LevelVar)
	switch {
	case trace:
		level.Set(LevelTrace)
	case debug:
		level.Set(slog.LevelDebug)
	default:
		level.Set(slog.LevelInfo)
	}

	slog.SetDefault(slog.New(newPrettyHandler(os.Stdout, level)))
}

// Fatal logs msg at error level with args, then exits the process.
func Fatal(msg string, args ...any) {
	slog.Error(msg, args...)
	os.Exit(1)
}

// Trace logs msg at LevelTrace.
func Trace(msg string, args ...any) {
	slog.Log(context.Background(), LevelTrace, msg, args...)
}

const (
	colorReset  = "\x1b[0m"
	colorDim    = "\x1b[2m"
	colorGray   = "\x1b[90m"
	colorRed    = "\x1b[31m"
	colorGreen  = "\x1b[32m"
	colorYellow = "\x1b[33m"
	colorCyan   = "\x1b[36m"
	colorBlue   = "\x1b[34m"
)

// prettyHandler is a colorized, single-line-per-record slog.Handler.
type prettyHandler struct {
	w      io.Writer
	level  slog.Leveler
	attrs  []slog.Attr
	groups []string
	mu     *sync.Mutex
}

// newPrettyHandler creates a pretty handler writing to w, filtered by level.
func newPrettyHandler(w io.Writer, level slog.Leveler) slog.Handler {
	return &prettyHandler{w: w, level: level, mu: &sync.Mutex{}}
}

// Enabled reports whether level is at or above the handler's minimum level.
func (h *prettyHandler) Enabled(_ context.Context, lvl slog.Level) bool {
	return lvl >= h.level.Level()
}

// Handle formats and writes one log record.
func (h *prettyHandler) Handle(_ context.Context, r slog.Record) error {
	ts := r.Time
	if ts.IsZero() {
		ts = time.Now()
	}

	levelText, levelColor := levelStyle(r.Level)
	var b strings.Builder
	b.WriteString(colorGray)
	b.WriteString(ts.Format("2006-01-02 15:04:05.000"))
	b.WriteString(colorReset)
	b.WriteByte(' ')
	b.WriteString(levelColor)
	b.WriteByte('[')
	b.WriteString(levelText)
	b.WriteByte(']')
	b.WriteString(colorReset)
	b.WriteByte(' ')
	b.WriteString(r.Message)

	attrs := make([]slog.Attr, 0, len(h.attrs)+r.NumAttrs())
	attrs = append(attrs, h.attrs...)
	r.Attrs(func(a slog.Attr) bool {
		attrs = append(attrs, a)
		return true
	})
	for _, a := range attrs {
		h.appendAttr(&b, h.groups, a)
	}
	b.WriteByte('\n')

	h.mu.Lock()
	defer h.mu.Unlock()
	_, err := io.WriteString(h.w, b.String())
	return err
}

// WithAttrs returns a handler that includes attrs on every record.
func (h *prettyHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	cloned := *h
	cloned.attrs = append(append([]slog.Attr{}, h.attrs...), attrs...)
	return &cloned
}

// WithGroup returns a handler that nests subsequent attrs under name.
func (h *prettyHandler) WithGroup(name string) slog.Handler {
	cloned := *h
	if strings.TrimSpace(name) != "" {
		cloned.groups = append(append([]string{}, h.groups...), name)
	}
	return &cloned
}

// appendAttr writes one key value pair or a flattened group to b.
func (h *prettyHandler) appendAttr(b *strings.Builder, groups []string, attr slog.Attr) {
	attr.Value = attr.Value.Resolve()
	if attr.Equal(slog.Attr{}) {
		return
	}

	if attr.Value.Kind() == slog.KindGroup {
		nextGroups := groups
		if attr.Key != "" {
			nextGroups = append(append([]string{}, groups...), attr.Key)
		}
		for _, nested := range attr.Value.Group() {
			h.appendAttr(b, nextGroups, nested)
		}
		return
	}

	keyParts := append(append([]string{}, groups...), attr.Key)
	key := strings.Join(keyParts, ".")
	if key == "" {
		return
	}

	b.WriteByte(' ')
	b.WriteString(colorDim)
	b.WriteString(key)
	b.WriteString(colorReset)
	b.WriteByte('=')
	b.WriteString(formatValue(attr.Value))
}

// formatValue renders a single slog value for the pretty printer.
func formatValue(v slog.Value) string {
	switch v.Kind() {
	case slog.KindString:
		s := v.String()
		if strings.ContainsAny(s, " \t\n\r") {
			return strconv.Quote(s)
		}
		return s
	case slog.KindTime:
		return v.Time().Format(time.RFC3339Nano)
	default:
		return fmt.Sprintf("%v", v.Any())
	}
}

// levelStyle returns the label and color for level.
func levelStyle(lvl slog.Level) (string, string) {
	switch {
	case lvl < slog.LevelDebug:
		return "TRACE", colorBlue
	case lvl < slog.LevelInfo:
		return "DEBUG", colorCyan
	case lvl < slog.LevelWarn:
		return "INFO", colorGreen
	case lvl < slog.LevelError:
		return "WARN", colorYellow
	default:
		return "ERROR", colorRed
	}
}
