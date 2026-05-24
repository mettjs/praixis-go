package internal

import (
	"io"
	"strings"
	"testing"
)

func newBody(s string) io.ReadCloser {
	return io.NopCloser(strings.NewReader(s))
}

func collectTokens(sr *StreamReader) ([]string, error) {
	var tokens []string
	for sr.Next() {
		tokens = append(tokens, sr.Token())
	}
	return tokens, sr.Err()
}

func TestStreamReader_ChatFormat(t *testing.T) {
	// Simulate: [SESSION_ID:abc123]\n followed by token chunks
	body := newBody("[SESSION_ID:abc123]\nHello, world!")
	sr := NewStreamReader(body)
	defer sr.Close()

	if got := sr.Meta("SESSION_ID"); got != "abc123" {
		t.Errorf("SESSION_ID = %q, want %q", got, "abc123")
	}

	tokens, err := collectTokens(sr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	joined := strings.Join(tokens, "")
	if joined != "Hello, world!" {
		t.Errorf("tokens joined = %q, want %q", joined, "Hello, world!")
	}
}

func TestStreamReader_RAGFormat(t *testing.T) {
	// Simulate: SESSION_ID + SEARCH_QUERY + SOURCES, then tokens
	body := newBody("[SESSION_ID:sess1]\n[SEARCH_QUERY:what is the policy]\n[SOURCES:doc1.pdf,doc2.docx]\nThe policy states...")
	sr := NewStreamReader(body)
	defer sr.Close()

	if got := sr.Meta("SESSION_ID"); got != "sess1" {
		t.Errorf("SESSION_ID = %q, want %q", got, "sess1")
	}
	if got := sr.Meta("SEARCH_QUERY"); got != "what is the policy" {
		t.Errorf("SEARCH_QUERY = %q, want %q", got, "what is the policy")
	}
	if got := sr.Meta("SOURCES"); got != "doc1.pdf,doc2.docx" {
		t.Errorf("SOURCES = %q, want %q", got, "doc1.pdf,doc2.docx")
	}

	tokens, err := collectTokens(sr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Join(tokens, "") != "The policy states..." {
		t.Errorf("unexpected token content: %q", strings.Join(tokens, ""))
	}
}

func TestStreamReader_FileSummaryFormat(t *testing.T) {
	// FILE prefix + PROGRESS lines, then tokens
	body := newBody("[FILE:report.pdf]\n[PROGRESS:mapping 3 chunks]\n[PROGRESS:reducing 3 chunks]\nHere is the summary.")
	sr := NewStreamReader(body)
	defer sr.Close()

	if got := sr.Meta("FILE"); got != "report.pdf" {
		t.Errorf("FILE = %q, want %q", got, "report.pdf")
	}
	if got := sr.Meta("PROGRESS"); got == "" {
		t.Error("expected PROGRESS metadata to be set")
	}

	tokens, err := collectTokens(sr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Join(tokens, "") != "Here is the summary." {
		t.Errorf("unexpected token content: %q", strings.Join(tokens, ""))
	}
}

func TestStreamReader_NoMetadata(t *testing.T) {
	// Stream with no metadata lines at all
	body := newBody("Just tokens, no metadata.")
	sr := NewStreamReader(body)
	defer sr.Close()

	if len(sr.AllMeta()) != 0 {
		t.Errorf("expected no metadata, got %v", sr.AllMeta())
	}

	tokens, err := collectTokens(sr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Join(tokens, "") != "Just tokens, no metadata." {
		t.Errorf("unexpected token content: %q", strings.Join(tokens, ""))
	}
}

func TestStreamReader_TokensWithNewlines(t *testing.T) {
	// LLM output can contain newlines; they should not be mistaken for metadata
	body := newBody("[SESSION_ID:s1]\nLine one\nLine two\n[not metadata because lowercase]\nLine three")
	sr := NewStreamReader(body)
	defer sr.Close()

	if sr.Meta("SESSION_ID") != "s1" {
		t.Errorf("SESSION_ID = %q, want %q", sr.Meta("SESSION_ID"), "s1")
	}

	tokens, err := collectTokens(sr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Once the first non-metadata byte is seen, everything remaining is tokens
	got := strings.Join(tokens, "")
	want := "Line one\nLine two\n[not metadata because lowercase]\nLine three"
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestStreamReader_EmptyStream(t *testing.T) {
	body := newBody("")
	sr := NewStreamReader(body)
	defer sr.Close()

	if sr.Next() {
		t.Error("Next() should return false on empty stream")
	}
	if sr.Err() != nil {
		t.Errorf("unexpected error on empty stream: %v", sr.Err())
	}
}

func TestStreamReader_MetadataOnly(t *testing.T) {
	// Metadata lines but no token content after
	body := newBody("[SESSION_ID:x]\n")
	sr := NewStreamReader(body)
	defer sr.Close()

	if sr.Meta("SESSION_ID") != "x" {
		t.Errorf("SESSION_ID = %q, want %q", sr.Meta("SESSION_ID"), "x")
	}
	if sr.Next() {
		t.Errorf("expected no tokens, got %q", sr.Token())
	}
	if sr.Err() != nil {
		t.Errorf("unexpected error: %v", sr.Err())
	}
}

func TestStreamReader_AllMeta_ReturnsCopy(t *testing.T) {
	body := newBody("[SESSION_ID:abc]\ntoken")
	sr := NewStreamReader(body)
	defer sr.Close()

	m1 := sr.AllMeta()
	m1["SESSION_ID"] = "mutated"

	if sr.Meta("SESSION_ID") != "abc" {
		t.Error("AllMeta should return a copy, not a reference to internal map")
	}
}
