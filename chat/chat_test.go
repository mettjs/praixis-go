package chat_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mettjs/praixis-go/chat"
	"github.com/mettjs/praixis-go/internal"
)

func newTestClient(serverURL string) *chat.Client {
	cfg := internal.NewConfig(serverURL, 5*time.Second)
	return chat.New(cfg, "test-key")
}

// ---- ChatStream ------------------------------------------------------------

func TestStream_SessionID_And_Tokens(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-Key") != "test-key" {
			t.Error("missing or wrong X-API-Key header")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "[SESSION_ID:sess-abc]\nHello, ")
		io.WriteString(w, "world!")
	}))
	defer srv.Close()

	stream, err := newTestClient(srv.URL).Stream(context.Background(), chat.Request{Prompt: "hi"})
	if err != nil {
		t.Fatalf("Stream() error: %v", err)
	}
	defer stream.Close()

	if stream.SessionID() != "sess-abc" {
		t.Errorf("SessionID() = %q, want %q", stream.SessionID(), "sess-abc")
	}

	var sb strings.Builder
	for stream.Next() {
		sb.WriteString(stream.Token())
	}
	if err := stream.Err(); err != nil {
		t.Fatalf("stream error: %v", err)
	}
	if sb.String() != "Hello, world!" {
		t.Errorf("tokens = %q, want %q", sb.String(), "Hello, world!")
	}
}

func TestStream_ContinuesExistingSession(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "[SESSION_ID:existing-sess]\nok")
	}))
	defer srv.Close()

	stream, err := newTestClient(srv.URL).Stream(context.Background(), chat.Request{
		Prompt:    "follow-up",
		SessionID: "existing-sess",
	})
	if err != nil {
		t.Fatalf("Stream() error: %v", err)
	}
	stream.Close()

	if gotBody["session_id"] != "existing-sess" {
		t.Errorf("session_id in body = %v, want %q", gotBody["session_id"], "existing-sess")
	}
}

func TestStream_GPUBusy_Returns503(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{"detail": "GPU slots exhausted"})
	}))
	defer srv.Close()

	_, err := newTestClient(srv.URL).Stream(context.Background(), chat.Request{Prompt: "hi"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !internal.IsGPUBusy(err) {
		t.Errorf("expected IsGPUBusy, got %v", err)
	}
}

func TestStream_RateLimit_Returns429(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		json.NewEncoder(w).Encode(map[string]string{"detail": "rate limit exceeded"})
	}))
	defer srv.Close()

	_, err := newTestClient(srv.URL).Stream(context.Background(), chat.Request{Prompt: "hi"})
	if !internal.IsRateLimit(err) {
		t.Errorf("expected IsRateLimit, got %v", err)
	}
}

// ---- FileSummaryStream -----------------------------------------------------

func TestSummarizeFile_Metadata_And_Tokens(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseMultipartForm(1 << 20)
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "[FILE:report.pdf]\n[PROGRESS:reducing 3 chunks]\nSummary text here.")
	}))
	defer srv.Close()

	stream, err := newTestClient(srv.URL).SummarizeFile(
		context.Background(), "report.pdf", strings.NewReader("doc content"), nil,
	)
	if err != nil {
		t.Fatalf("SummarizeFile() error: %v", err)
	}
	defer stream.Close()

	if stream.Filename() != "report.pdf" {
		t.Errorf("Filename() = %q, want %q", stream.Filename(), "report.pdf")
	}
	if stream.Progress() != "reducing 3 chunks" {
		t.Errorf("Progress() = %q, want %q", stream.Progress(), "reducing 3 chunks")
	}
	if stream.BackendError() != "" {
		t.Errorf("unexpected BackendError(): %q", stream.BackendError())
	}

	var sb strings.Builder
	for stream.Next() {
		sb.WriteString(stream.Token())
	}
	if sb.String() != "Summary text here." {
		t.Errorf("tokens = %q, want %q", sb.String(), "Summary text here.")
	}
}

func TestSummarizeFile_BackendError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "[FILE:doc.txt]\n[ERROR:GPU slots exhausted, try again later]\n")
	}))
	defer srv.Close()

	stream, err := newTestClient(srv.URL).SummarizeFile(
		context.Background(), "doc.txt", strings.NewReader("text"), nil,
	)
	if err != nil {
		t.Fatalf("SummarizeFile() error: %v", err)
	}
	defer stream.Close()

	// Drain the (empty) token stream
	for stream.Next() {
	}

	if stream.BackendError() == "" {
		t.Error("expected BackendError to be set")
	}
	if stream.Err() != nil {
		t.Errorf("unexpected IO error: %v", stream.Err())
	}
}

func TestSummarizeFile_SendsCustomOptions(t *testing.T) {
	var gotTask, gotTone string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseMultipartForm(1 << 20)
		gotTask = r.FormValue("task")
		gotTone = r.FormValue("tone")
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "[FILE:f.txt]\nok")
	}))
	defer srv.Close()

	stream, err := newTestClient(srv.URL).SummarizeFile(
		context.Background(), "f.txt", strings.NewReader("data"),
		&chat.FileSummaryOptions{Task: "Extract action items", Tone: "Casual"},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	stream.Close()

	if gotTask != "Extract action items" {
		t.Errorf("task = %q, want %q", gotTask, "Extract action items")
	}
	if gotTone != "Casual" {
		t.Errorf("tone = %q, want %q", gotTone, "Casual")
	}
}

// ---- History ---------------------------------------------------------------

func TestHistory_ReturnsMessages(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %q, want GET", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/sess-1") {
			t.Errorf("path = %q, want suffix /sess-1", r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]any{
			"session_id": "sess-1",
			"history": []map[string]string{
				{"role": "user", "content": "hello"},
				{"role": "assistant", "content": "hi there"},
			},
		})
	}))
	defer srv.Close()

	h, err := newTestClient(srv.URL).History(context.Background(), "sess-1")
	if err != nil {
		t.Fatalf("History() error: %v", err)
	}
	if h.SessionID != "sess-1" {
		t.Errorf("SessionID = %q, want %q", h.SessionID, "sess-1")
	}
	if len(h.History) != 2 {
		t.Errorf("len(History) = %d, want 2", len(h.History))
	}
	if h.History[0].Role != "user" || h.History[0].Content != "hello" {
		t.Errorf("History[0] = %+v", h.History[0])
	}
}

func TestHistory_EmptySessionID_Error(t *testing.T) {
	_, err := newTestClient("http://localhost").History(context.Background(), "")
	if err == nil {
		t.Error("expected error for empty sessionID")
	}
}

// ---- ActiveSessions --------------------------------------------------------

func TestActiveSessions_ReturnsList(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"active_sessions": []string{"s1", "s2", "s3"},
		})
	}))
	defer srv.Close()

	sessions, err := newTestClient(srv.URL).ActiveSessions(context.Background())
	if err != nil {
		t.Fatalf("ActiveSessions() error: %v", err)
	}
	if len(sessions) != 3 {
		t.Errorf("len = %d, want 3", len(sessions))
	}
	if sessions[1] != "s2" {
		t.Errorf("sessions[1] = %q, want %q", sessions[1], "s2")
	}
}

// ---- DeleteSession ---------------------------------------------------------

func TestDeleteSession_SendsDELETE(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		json.NewEncoder(w).Encode(map[string]string{"status": "success"})
	}))
	defer srv.Close()

	err := newTestClient(srv.URL).DeleteSession(context.Background(), "sess-42")
	if err != nil {
		t.Fatalf("DeleteSession() error: %v", err)
	}
	if gotMethod != http.MethodDelete {
		t.Errorf("method = %q, want DELETE", gotMethod)
	}
	if !strings.HasSuffix(gotPath, "/sess-42") {
		t.Errorf("path = %q, want suffix /sess-42", gotPath)
	}
}

func TestDeleteSession_EmptySessionID_Error(t *testing.T) {
	err := newTestClient("http://localhost").DeleteSession(context.Background(), "")
	if err == nil {
		t.Error("expected error for empty sessionID")
	}
}
