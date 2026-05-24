// Package internal provides the low-level HTTP and stream-parsing primitives
// shared across all SDK resource clients.
package internal

import (
	"bufio"
	"io"
	"regexp"
	"strings"
)

// metaLineRe matches lines of the form [KEY:value] where KEY is uppercase letters
// and underscores. These are Praixis metadata prefix lines, not LLM token content.
var metaLineRe = regexp.MustCompile(`^\[([A-Z_]+):([^\]]*)\]$`)

// StreamReader reads a raw text/event-stream response from the Praixis backend.
// StreamReader is not safe for concurrent use; each stream must be consumed by
// a single goroutine.
//
// The backend yields metadata lines first (e.g. [SESSION_ID:...], [SOURCES:...])
// followed by raw LLM token chunks. StreamReader consumes all leading metadata
// eagerly on construction, then exposes tokens via the Next/Token iterator.
//
// Typical usage:
//
//	sr := internal.NewStreamReader(respBody)
//	defer sr.Close()
//
//	sessionID := sr.Meta("SESSION_ID")
//	for sr.Next() {
//	    fmt.Print(sr.Token())
//	}
//	if err := sr.Err(); err != nil { ... }
type StreamReader struct {
	body    io.ReadCloser
	br      *bufio.Reader
	meta    map[string]string
	buf     []byte // reused across Next() calls to avoid per-chunk allocations
	pending string // bytes that were read during metadata scan but belong to token content
	token   string // current token chunk; valid only after Next() returns true
	done    bool
	err     error
}

// NewStreamReader constructs a StreamReader from an open HTTP response body and
// immediately reads all leading metadata lines into an internal map.
// The caller must call Close() when done.
func NewStreamReader(body io.ReadCloser) *StreamReader {
	r := &StreamReader{
		body: body,
		br:   bufio.NewReaderSize(body, 4096),
		meta: make(map[string]string),
		buf:  make([]byte, 4096),
	}
	r.consumeMetadata()
	return r
}

// Meta returns the value for a metadata key parsed from the stream prefix.
// Keys are uppercase, e.g. "SESSION_ID", "SEARCH_QUERY", "SOURCES", "PROGRESS", "FILE", "ERROR".
// Returns "" if the key was not present.
func (r *StreamReader) Meta(key string) string {
	return r.meta[key]
}

// AllMeta returns a copy of every metadata key-value pair from the stream prefix.
func (r *StreamReader) AllMeta() map[string]string {
	out := make(map[string]string, len(r.meta))
	for k, v := range r.meta {
		out[k] = v
	}
	return out
}

// Next advances to the next token chunk. Returns true if a token is available,
// false when the stream is exhausted or an error occurred.
// After Next() returns false, check Err() to distinguish clean EOF from error.
func (r *StreamReader) Next() bool {
	if r.done || r.err != nil {
		return false
	}

	// Flush any byte(s) that were buffered during the metadata scan
	// (the first non-metadata byte that was peeked but not consumed as metadata).
	if r.pending != "" {
		r.token = r.pending
		r.pending = ""
		return true
	}

	n, err := r.br.Read(r.buf)
	if n > 0 {
		r.token = string(r.buf[:n])
		if err == io.EOF {
			r.done = true
		}
		return true
	}
	if err == io.EOF {
		r.done = true
		return false
	}
	r.err = err
	return false
}

// Token returns the current token chunk. Only valid after Next() returns true.
func (r *StreamReader) Token() string { return r.token }

// Err returns the first non-EOF error that stopped the stream, or nil.
func (r *StreamReader) Err() error { return r.err }

// Close closes the underlying HTTP response body.
func (r *StreamReader) Close() error { return r.body.Close() }

// consumeMetadata reads and stores all leading [KEY:VALUE]\n lines from the stream.
// The first byte sequence that does not match a metadata line is stored in r.pending
// so that Next() can yield it as the first token chunk.
func (r *StreamReader) consumeMetadata() {
	for {
		b, err := r.br.ReadByte()
		if err != nil {
			if err != io.EOF {
				r.err = err
			}
			return
		}

		if b != '[' {
			// Definitely not a metadata line; buffer this byte and stop.
			r.pending = string([]byte{b})
			return
		}

		// Read until newline so we can test the full candidate line.
		line, readErr := r.br.ReadString('\n')
		candidate := "[" + strings.TrimSuffix(line, "\n")

		m := metaLineRe.FindStringSubmatch(candidate)
		if m == nil {
			// Starts with '[' but is not a metadata line — treat as token content.
			// Only restore the newline if ReadString actually consumed one.
			if readErr == nil {
				r.pending = candidate + "\n"
			} else {
				r.pending = candidate
				if readErr != io.EOF {
					r.err = readErr
				}
			}
			return
		}

		r.meta[m[1]] = m[2]

		if readErr != nil {
			if readErr != io.EOF {
				r.err = readErr
			}
			return
		}
	}
}
