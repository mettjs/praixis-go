package internal

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func testConfig(serverURL string) Config {
	return NewConfig(serverURL, 5*time.Second)
}

// ---- DoJSON ----------------------------------------------------------------

func TestDoJSON_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	defer srv.Close()

	var dst map[string]string
	err := DoJSON(context.Background(), testConfig(srv.URL), http.MethodGet, "/ping", nil, nil, &dst)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dst["status"] != "ok" {
		t.Errorf("status = %q, want %q", dst["status"], "ok")
	}
}

func TestDoJSON_SendsCustomHeaders(t *testing.T) {
	var gotKey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("X-API-Key")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	DoJSON(context.Background(), testConfig(srv.URL), http.MethodGet, "/", map[string]string{"X-API-Key": "test-key"}, nil, nil)
	if gotKey != "test-key" {
		t.Errorf("X-API-Key = %q, want %q", gotKey, "test-key")
	}
}

func TestDoJSON_APIError_Detail_String(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]string{"detail": "invalid api key"})
	}))
	defer srv.Close()

	err := DoJSON(context.Background(), testConfig(srv.URL), http.MethodGet, "/", nil, nil, nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	ae, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected *APIError, got %T", err)
	}
	if ae.StatusCode != http.StatusForbidden {
		t.Errorf("StatusCode = %d, want %d", ae.StatusCode, http.StatusForbidden)
	}
	if ae.Message != "invalid api key" {
		t.Errorf("Message = %q, want %q", ae.Message, "invalid api key")
	}
}

func TestDoJSON_APIError_RawBody_Fallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal server error"))
	}))
	defer srv.Close()

	err := DoJSON(context.Background(), testConfig(srv.URL), http.MethodGet, "/", nil, nil, nil)
	ae, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected *APIError, got %T", err)
	}
	if ae.StatusCode != http.StatusInternalServerError {
		t.Errorf("StatusCode = %d", ae.StatusCode)
	}
	if ae.Message != "internal server error" {
		t.Errorf("Message = %q", ae.Message)
	}
}

// ---- Error helpers ---------------------------------------------------------

func TestIsRateLimit(t *testing.T) {
	if !IsRateLimit(&APIError{StatusCode: 429}) {
		t.Error("expected IsRateLimit true for 429")
	}
	if IsRateLimit(&APIError{StatusCode: 403}) {
		t.Error("expected IsRateLimit false for 403")
	}
	if IsRateLimit(nil) {
		t.Error("expected IsRateLimit false for nil")
	}
}

func TestIsUnauthorized(t *testing.T) {
	if !IsUnauthorized(&APIError{StatusCode: 401}) {
		t.Error("expected IsUnauthorized true for 401")
	}
	if !IsUnauthorized(&APIError{StatusCode: 403}) {
		t.Error("expected IsUnauthorized true for 403")
	}
	if IsUnauthorized(&APIError{StatusCode: 429}) {
		t.Error("expected IsUnauthorized false for 429")
	}
}

func TestIsGPUBusy(t *testing.T) {
	if !IsGPUBusy(&APIError{StatusCode: 503}) {
		t.Error("expected IsGPUBusy true for 503")
	}
	if IsGPUBusy(&APIError{StatusCode: 429}) {
		t.Error("expected IsGPUBusy false for 429")
	}
}

// ---- DoStream --------------------------------------------------------------

func TestDoStream_ReturnsBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte("[SESSION_ID:s1]\nHello"))
	}))
	defer srv.Close()

	body, err := DoStream(context.Background(), testConfig(srv.URL), http.MethodPost, "/stream", nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer body.Close()

	sr := NewStreamReader(body)
	if sr.Meta("SESSION_ID") != "s1" {
		t.Errorf("SESSION_ID = %q", sr.Meta("SESSION_ID"))
	}
}

func TestDoStream_ErrorResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{"detail": "GPU slots exhausted"})
	}))
	defer srv.Close()

	_, err := DoStream(context.Background(), testConfig(srv.URL), http.MethodPost, "/stream", nil, nil)
	if !IsGPUBusy(err) {
		t.Errorf("expected IsGPUBusy, got %v", err)
	}
}

// ---- DoMultipart -----------------------------------------------------------

func TestDoMultipart_SendsFieldsAndFile(t *testing.T) {
	var gotCollection, gotFilename string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseMultipartForm(10 << 20)
		gotCollection = r.FormValue("collection_name")
		_, h, _ := r.FormFile("files")
		if h != nil {
			gotFilename = h.Filename
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	defer srv.Close()

	var dst map[string]string
	err := DoMultipart(context.Background(), testConfig(srv.URL), "/upload", nil,
		map[string]string{"collection_name": "docs"},
		[]FileAttachment{{FieldName: "files", Filename: "test.txt", Data: strings.NewReader("hello")}},
		&dst,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotCollection != "docs" {
		t.Errorf("collection_name = %q, want %q", gotCollection, "docs")
	}
	if gotFilename != "test.txt" {
		t.Errorf("filename = %q, want %q", gotFilename, "test.txt")
	}
}

// ---- mimeByExtension -------------------------------------------------------

func TestMimeByExtension(t *testing.T) {
	cases := []struct{ file, want string }{
		{"doc.pdf", "application/pdf"},
		{"doc.PDF", "application/pdf"},
		{"doc.docx", "application/vnd.openxmlformats-officedocument.wordprocessingml.document"},
		{"doc.txt", "text/plain"},
		{"doc.csv", "application/octet-stream"},
	}
	for _, c := range cases {
		got := mimeByExtension(c.file)
		if got != c.want {
			t.Errorf("mimeByExtension(%q) = %q, want %q", c.file, got, c.want)
		}
	}
}
