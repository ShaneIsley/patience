package run

import (
	"bytes"
	"io"
	"regexp"
	"sync"
)

// maxLine bounds the bytes held while waiting for a newline. A longer line is
// matched in maxLine-sized chunks, so memory stays flat on any output and a
// pattern can only be missed if it straddles a chunk boundary of a line that
// long.
const maxLine = 64 * 1024

// matcher records whether the success and failure patterns matched anywhere
// in the streamed output. It is shared by the stdout and stderr writers.
type matcher struct {
	success, failure *regexp.Regexp
	mu               sync.Mutex
	sawSuccess       bool
	sawFailure       bool
}

func (m *matcher) active() bool { return m != nil && (m.success != nil || m.failure != nil) }

func (m *matcher) test(line []byte) {
	if len(line) == 0 {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failure != nil && !m.sawFailure && m.failure.Match(line) {
		m.sawFailure = true
	}
	if m.success != nil && !m.sawSuccess && m.success.Match(line) {
		m.sawSuccess = true
	}
}

// scanWriter passes every byte through to dst unchanged and feeds complete
// lines to the matcher. It never blocks the child on pattern work beyond the
// regexp itself and never buffers more than maxLine bytes.
type scanWriter struct {
	dst io.Writer
	m   *matcher
	buf []byte
}

func newScanWriter(dst io.Writer, m *matcher) io.Writer {
	if !m.active() {
		return dst
	}
	return &scanWriter{dst: dst, m: m}
}

func (w *scanWriter) Write(p []byte) (int, error) {
	n, err := w.dst.Write(p)
	// Scan what was accepted; a short write is the destination's problem,
	// not a reason to lose the pattern state.
	data := p[:n]
	for len(data) > 0 {
		i := bytes.IndexByte(data, '\n')
		if i < 0 {
			w.buf = append(w.buf, data...)
			if len(w.buf) >= maxLine {
				w.m.test(w.buf)
				w.buf = w.buf[:0]
			}
			break
		}
		line := data[:i]
		if len(w.buf) > 0 {
			line = append(w.buf, line...)
			w.buf = w.buf[:0]
		}
		w.m.test(line)
		data = data[i+1:]
	}
	return n, err
}

// flush tests any trailing partial line once the stream is finished.
func (w *scanWriter) flush() {
	if len(w.buf) > 0 {
		w.m.test(w.buf)
		w.buf = w.buf[:0]
	}
}
