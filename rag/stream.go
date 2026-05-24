package rag

import (
	"strings"

	"github.com/mettjs/praixis-go/internal"
)

// AskStream is the result of Client.Ask. It exposes the three metadata fields
// emitted before tokens ([SESSION_ID:...], [SEARCH_QUERY:...], [SOURCES:...])
// and then iterates over LLM token chunks.
//
// Always close the stream when done, even on error:
//
//	defer stream.Close()
type AskStream struct {
	sr *internal.StreamReader
}

func newAskStream(sr *internal.StreamReader) *AskStream {
	return &AskStream{sr: sr}
}

// SessionID returns the session ID assigned or reused by the server.
// Available immediately after Client.Ask returns, before the first Next() call.
func (s *AskStream) SessionID() string { return s.sr.Meta("SESSION_ID") }

// SearchQuery returns the (possibly reformulated) query the server used to
// retrieve context chunks. Useful for debugging retrieval quality.
func (s *AskStream) SearchQuery() string { return s.sr.Meta("SEARCH_QUERY") }

// Sources returns the unique filenames that contributed context to this answer.
// The server emits them as a comma-separated [SOURCES:...] line; this method
// splits them into a slice. Returns nil if no sources were emitted.
func (s *AskStream) Sources() []string {
	raw := s.sr.Meta("SOURCES")
	if raw == "" {
		return nil
	}
	return strings.Split(raw, ",")
}

// Next advances to the next token chunk. Returns false when the stream ends
// or an error occurs. Check Err() after a false return.
func (s *AskStream) Next() bool { return s.sr.Next() }

// Token returns the current token chunk. Only valid after Next() returns true.
func (s *AskStream) Token() string { return s.sr.Token() }

// Err returns the first non-EOF IO or network error that stopped the stream.
func (s *AskStream) Err() error { return s.sr.Err() }

// Close closes the underlying HTTP response body.
func (s *AskStream) Close() error { return s.sr.Close() }
