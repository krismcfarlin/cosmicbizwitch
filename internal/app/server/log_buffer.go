package server

import (
	"bytes"
	"io"
	"sync"
	"time"
)

const maxLogLines = 2000

// LogLine holds a single captured log line with a timestamp
type LogLine struct {
	Time string
	Text string
}

// LogBuffer is a thread-safe ring buffer that captures log output line-by-line
type LogBuffer struct {
	mu    sync.RWMutex
	lines []LogLine
	buf   bytes.Buffer // partial line accumulator
}

// NewLogBuffer creates a new LogBuffer
func NewLogBuffer() *LogBuffer {
	return &LogBuffer{
		lines: make([]LogLine, 0, maxLogLines),
	}
}

// Write implements io.Writer. Lines are split on newlines and stored.
func (b *LogBuffer) Write(p []byte) (n int, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.buf.Write(p)
	for {
		data := b.buf.Bytes()
		idx := bytes.IndexByte(data, '\n')
		if idx < 0 {
			break
		}
		line := string(data[:idx])
		b.buf.Next(idx + 1)

		b.lines = append(b.lines, LogLine{
			Time: time.Now().Format("2006-01-02 15:04:05"),
			Text: line,
		})
		if len(b.lines) > maxLogLines {
			b.lines = b.lines[len(b.lines)-maxLogLines:]
		}
	}
	return len(p), nil
}

// Lines returns up to n most-recent lines matching filter (case-insensitive substring).
func (b *LogBuffer) Lines(n int, filter string) []LogLine {
	b.mu.RLock()
	defer b.mu.RUnlock()

	src := b.lines

	// Apply filter first
	if filter != "" {
		lower := toLower(filter)
		var filtered []LogLine
		for _, l := range src {
			if containsLower(l.Text, lower) {
				filtered = append(filtered, l)
			}
		}
		src = filtered
	}

	if n <= 0 || n >= len(src) {
		return append([]LogLine(nil), src...)
	}
	return append([]LogLine(nil), src[len(src)-n:]...)
}

// TeeWriter returns an io.Writer that writes to both dst and the buffer.
func (b *LogBuffer) TeeWriter(dst io.Writer) io.Writer {
	return io.MultiWriter(dst, b)
}

// simple helpers to avoid importing strings in this file
func toLower(s string) string {
	out := make([]byte, len(s))
	for i := range s {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 32
		}
		out[i] = c
	}
	return string(out)
}

func containsLower(text, lowerFilter string) bool {
	return len(lowerFilter) == 0 || len(toLower(text)) >= len(lowerFilter) &&
		func() bool {
			t := toLower(text)
			f := lowerFilter
			for i := 0; i <= len(t)-len(f); i++ {
				if t[i:i+len(f)] == f {
					return true
				}
			}
			return false
		}()
}
