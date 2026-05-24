package chat

import (
	"github.com/mettjs/praixis-go/internal"
)

// ChatStream is the result of Client.Stream. It exposes the session ID from the
// stream prefix and iterates over LLM token chunks as they arrive.
//
// Always close the stream when done, even on error:
//
//	defer stream.Close()
type ChatStream struct {
	sr *internal.StreamReader
}

func newChatStream(sr *internal.StreamReader) *ChatStream {
	return &ChatStream{sr: sr}
}

// SessionID returns the session ID emitted as the first metadata line by the
// server. It is populated immediately — before the first call to Next().
// It will be a new ID when no session_id was passed in the request.
func (s *ChatStream) SessionID() string { return s.sr.Meta("SESSION_ID") }

// Next advances to the next token chunk. Returns false when the stream ends
// or an error occurs. Check Err() after a false return.
func (s *ChatStream) Next() bool { return s.sr.Next() }

// Token returns the current token chunk. Only valid after Next() returns true.
func (s *ChatStream) Token() string { return s.sr.Token() }

// Err returns the first non-EOF IO or network error that stopped the stream.
func (s *ChatStream) Err() error { return s.sr.Err() }

// Close closes the underlying HTTP response body.
func (s *ChatStream) Close() error { return s.sr.Close() }

// FileSummaryStream is the result of Client.SummarizeFile.
//
// The server emits a [FILE:...] prefix and optional [PROGRESS:...] lines
// before any tokens. For multi-chunk documents a map-reduce pipeline runs
// server-side; progress reflects the current phase.
//
// If the server fails mid-stream (e.g. GPU slots exhausted) it emits an
// [ERROR:...] line. Check BackendError() after the token loop.
type FileSummaryStream struct {
	sr *internal.StreamReader
}

func newFileSummaryStream(sr *internal.StreamReader) *FileSummaryStream {
	return &FileSummaryStream{sr: sr}
}

// Filename returns the original filename echoed back by the server via [FILE:...].
func (s *FileSummaryStream) Filename() string { return s.sr.Meta("FILE") }

// Progress returns the latest [PROGRESS:...] value from the stream prefix,
// e.g. "mapping 5 chunks" or "reducing 5 chunks". Empty for single-chunk docs.
func (s *FileSummaryStream) Progress() string { return s.sr.Meta("PROGRESS") }

// BackendError returns the [ERROR:...] value when the server emitted a backend
// error (e.g. GPU busy). Check this after the token loop alongside Err().
func (s *FileSummaryStream) BackendError() string { return s.sr.Meta("ERROR") }

// Next advances to the next token chunk. Returns false when the stream ends
// or an error occurs. Check Err() and BackendError() after a false return.
func (s *FileSummaryStream) Next() bool { return s.sr.Next() }

// Token returns the current token chunk. Only valid after Next() returns true.
func (s *FileSummaryStream) Token() string { return s.sr.Token() }

// Err returns the first non-EOF IO or network error that stopped the stream.
func (s *FileSummaryStream) Err() error { return s.sr.Err() }

// Close closes the underlying HTTP response body.
func (s *FileSummaryStream) Close() error { return s.sr.Close() }
