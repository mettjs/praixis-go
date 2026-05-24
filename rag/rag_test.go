package rag_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mettjs/praixis-go/internal"
	"github.com/mettjs/praixis-go/rag"
)

func newTestClient(serverURL string) *rag.Client {
	return rag.New(internal.NewConfig(serverURL, 5*time.Second), "test-key")
}

func intPtr(v int) *int { return &v }

// ---- AskStream -------------------------------------------------------------

func TestAsk_Metadata_And_Tokens(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-Key") != "test-key" {
			t.Error("missing X-API-Key header")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "[SESSION_ID:s1]\n[SEARCH_QUERY:what is the policy on leave]\n[SOURCES:policy.pdf,hr.docx]\nYou may take up to 20 days.")
	}))
	defer srv.Close()

	stream, err := newTestClient(srv.URL).Ask(context.Background(), rag.QuestionRequest{
		CollectionName: "hr-docs",
		Question:       "how many leave days do I get",
	})
	if err != nil {
		t.Fatalf("Ask() error: %v", err)
	}
	defer stream.Close()

	if stream.SessionID() != "s1" {
		t.Errorf("SessionID() = %q, want %q", stream.SessionID(), "s1")
	}
	if stream.SearchQuery() != "what is the policy on leave" {
		t.Errorf("SearchQuery() = %q", stream.SearchQuery())
	}
	sources := stream.Sources()
	if len(sources) != 2 || sources[0] != "policy.pdf" || sources[1] != "hr.docx" {
		t.Errorf("Sources() = %v, want [policy.pdf hr.docx]", sources)
	}

	var sb strings.Builder
	for stream.Next() {
		sb.WriteString(stream.Token())
	}
	if err := stream.Err(); err != nil {
		t.Fatalf("stream error: %v", err)
	}
	if sb.String() != "You may take up to 20 days." {
		t.Errorf("tokens = %q", sb.String())
	}
}

func TestAsk_NoSources_ReturnsNil(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "[SESSION_ID:s2]\n[SEARCH_QUERY:q]\nno context found")
	}))
	defer srv.Close()

	stream, err := newTestClient(srv.URL).Ask(context.Background(), rag.QuestionRequest{
		CollectionName: "col", Question: "q",
	})
	if err != nil {
		t.Fatalf("Ask() error: %v", err)
	}
	defer stream.Close()
	for stream.Next() {
	}

	if stream.Sources() != nil {
		t.Errorf("expected nil Sources, got %v", stream.Sources())
	}
}

func TestAsk_GPUBusy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{"detail": "GPU slots exhausted"})
	}))
	defer srv.Close()

	_, err := newTestClient(srv.URL).Ask(context.Background(), rag.QuestionRequest{
		CollectionName: "col", Question: "q",
	})
	if !internal.IsGPUBusy(err) {
		t.Errorf("expected IsGPUBusy, got %v", err)
	}
}

// ---- Upload ----------------------------------------------------------------

func TestUpload_SingleFile_DefaultOptions(t *testing.T) {
	var gotCollection, gotChunkSize, gotChunkOverlap, gotFilename string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseMultipartForm(10 << 20)
		gotCollection = r.FormValue("collection_name")
		gotChunkSize = r.FormValue("chunk_size")
		gotChunkOverlap = r.FormValue("chunk_overlap")
		_, h, _ := r.FormFile("files")
		if h != nil {
			gotFilename = h.Filename
		}
		json.NewEncoder(w).Encode(rag.UploadResponse{
			CollectionName: "main",
			Processed:      1,
			Succeeded:      1,
			Results:        []rag.UploadResult{{Filename: "doc.pdf", Status: "success"}},
		})
	}))
	defer srv.Close()

	resp, err := newTestClient(srv.URL).Upload(context.Background(),
		[]rag.FileUpload{{Filename: "doc.pdf", Data: strings.NewReader("content")}},
		nil,
	)
	if err != nil {
		t.Fatalf("Upload() error: %v", err)
	}
	if resp.Succeeded != 1 {
		t.Errorf("Succeeded = %d, want 1", resp.Succeeded)
	}
	// nil opts → server defaults; no fields sent
	if gotCollection != "" {
		t.Errorf("collection_name should not be sent when opts is nil, got %q", gotCollection)
	}
	if gotChunkSize != "" {
		t.Errorf("chunk_size should not be sent when opts is nil, got %q", gotChunkSize)
	}
	if gotChunkOverlap != "" {
		t.Errorf("chunk_overlap should not be sent when opts is nil, got %q", gotChunkOverlap)
	}
	if gotFilename != "doc.pdf" {
		t.Errorf("filename = %q, want %q", gotFilename, "doc.pdf")
	}
}

func TestUpload_CustomOptions(t *testing.T) {
	var gotCollection, gotChunkSize, gotChunkOverlap string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseMultipartForm(10 << 20)
		gotCollection = r.FormValue("collection_name")
		gotChunkSize = r.FormValue("chunk_size")
		gotChunkOverlap = r.FormValue("chunk_overlap")
		json.NewEncoder(w).Encode(rag.UploadResponse{CollectionName: "policies", Processed: 1, Succeeded: 1})
	}))
	defer srv.Close()

	newTestClient(srv.URL).Upload(context.Background(),
		[]rag.FileUpload{{Filename: "f.txt", Data: strings.NewReader("x")}},
		&rag.UploadOptions{CollectionName: "policies", ChunkSize: intPtr(500), ChunkOverlap: intPtr(0)},
	)

	if gotCollection != "policies" {
		t.Errorf("collection_name = %q, want %q", gotCollection, "policies")
	}
	if gotChunkSize != "500" {
		t.Errorf("chunk_size = %q, want %q", gotChunkSize, "500")
	}
	// ChunkOverlap 0 is a valid value and must be sent
	if gotChunkOverlap != "0" {
		t.Errorf("chunk_overlap = %q, want %q", gotChunkOverlap, "0")
	}
}

func TestUpload_NoFiles_Error(t *testing.T) {
	_, err := newTestClient("http://localhost").Upload(context.Background(), nil, nil)
	if err == nil {
		t.Error("expected error for empty files slice")
	}
}

func TestUpload_MultipleFiles(t *testing.T) {
	var fileCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseMultipartForm(10 << 20)
		fileCount = len(r.MultipartForm.File["files"])
		json.NewEncoder(w).Encode(rag.UploadResponse{Processed: 2, Succeeded: 2})
	}))
	defer srv.Close()

	newTestClient(srv.URL).Upload(context.Background(), []rag.FileUpload{
		{Filename: "a.pdf", Data: strings.NewReader("a")},
		{Filename: "b.pdf", Data: strings.NewReader("b")},
	}, nil)

	if fileCount != 2 {
		t.Errorf("server received %d files, want 2", fileCount)
	}
}

// ---- Embed -----------------------------------------------------------------

func TestEmbed_ReturnsVector(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		if body["text"] != "hello world" {
			t.Errorf("text = %q, want %q", body["text"], "hello world")
		}
		json.NewEncoder(w).Encode(rag.EmbedResponse{
			Text:       "hello world",
			Dimensions: 3,
			Embedding:  []float64{0.1, 0.2, 0.3},
		})
	}))
	defer srv.Close()

	resp, err := newTestClient(srv.URL).Embed(context.Background(), "hello world")
	if err != nil {
		t.Fatalf("Embed() error: %v", err)
	}
	if resp.Dimensions != 3 {
		t.Errorf("Dimensions = %d, want 3", resp.Dimensions)
	}
	if len(resp.Embedding) != 3 {
		t.Errorf("len(Embedding) = %d, want 3", len(resp.Embedding))
	}
}

// ---- ListCollections -------------------------------------------------------

func TestListCollections(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/rag-db/list" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]any{
			"status": "success", "total_documents": 2,
			"active_collections": []string{"docs", "policies"},
		})
	}))
	defer srv.Close()

	list, err := newTestClient(srv.URL).ListCollections(context.Background())
	if err != nil {
		t.Fatalf("ListCollections() error: %v", err)
	}
	if list.TotalDocuments != 2 {
		t.Errorf("TotalDocuments = %d, want 2", list.TotalDocuments)
	}
	if len(list.ActiveCollections) != 2 || list.ActiveCollections[1] != "policies" {
		t.Errorf("ActiveCollections = %v", list.ActiveCollections)
	}
}

// ---- ListFiles -------------------------------------------------------------

func TestListFiles_EncodesCollectionInPath(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		json.NewEncoder(w).Encode(rag.FileList{
			CollectionName: "my-docs",
			TotalFiles:     1,
			FilesStored:    []string{"report.pdf"},
		})
	}))
	defer srv.Close()

	list, err := newTestClient(srv.URL).ListFiles(context.Background(), "my-docs")
	if err != nil {
		t.Fatalf("ListFiles() error: %v", err)
	}
	if gotPath != "/rag-db/my-docs/files" {
		t.Errorf("path = %q, want /rag-db/my-docs/files", gotPath)
	}
	if len(list.FilesStored) != 1 || list.FilesStored[0] != "report.pdf" {
		t.Errorf("FilesStored = %v", list.FilesStored)
	}
}

func TestListFiles_EmptyName_Error(t *testing.T) {
	_, err := newTestClient("http://localhost").ListFiles(context.Background(), "")
	if err == nil {
		t.Error("expected error for empty collectionName")
	}
}

// ---- DeleteCollection ------------------------------------------------------

func TestDeleteCollection_SendsDELETE(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		json.NewEncoder(w).Encode(map[string]string{"status": "success"})
	}))
	defer srv.Close()

	if err := newTestClient(srv.URL).DeleteCollection(context.Background(), "old-docs"); err != nil {
		t.Fatalf("DeleteCollection() error: %v", err)
	}
	if gotMethod != http.MethodDelete {
		t.Errorf("method = %q, want DELETE", gotMethod)
	}
	if gotPath != "/rag-db/delete/old-docs" {
		t.Errorf("path = %q, want /rag-db/delete/old-docs", gotPath)
	}
}

// ---- DeleteFile ------------------------------------------------------------

func TestDeleteFile_SendsDELETE_WithEncodedPath(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// EscapedPath returns the raw encoded form; Path is pre-decoded by net/http.
		gotPath = r.URL.EscapedPath()
		json.NewEncoder(w).Encode(map[string]string{"status": "success"})
	}))
	defer srv.Close()

	if err := newTestClient(srv.URL).DeleteFile(context.Background(), "docs", "report 2024.pdf"); err != nil {
		t.Fatalf("DeleteFile() error: %v", err)
	}
	if gotPath != "/rag-db/docs/files/report%202024.pdf" {
		t.Errorf("path = %q, want /rag-db/docs/files/report%%202024.pdf", gotPath)
	}
}

func TestDeleteFile_EmptyArgs_Error(t *testing.T) {
	c := newTestClient("http://localhost")
	if err := c.DeleteFile(context.Background(), "", "f.pdf"); err == nil {
		t.Error("expected error for empty collectionName")
	}
	if err := c.DeleteFile(context.Background(), "col", ""); err == nil {
		t.Error("expected error for empty filename")
	}
}

// ---- Summarize -------------------------------------------------------------

func TestSummarize_ReturnsText(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		json.NewEncoder(w).Encode(rag.Summary{
			Filename: "policy.pdf",
			Summary:  "This document outlines the leave policy.",
		})
	}))
	defer srv.Close()

	s, err := newTestClient(srv.URL).Summarize(context.Background(), "hr", "policy.pdf")
	if err != nil {
		t.Fatalf("Summarize() error: %v", err)
	}
	if gotPath != "/rag-db/knowledge_base/hr/files/policy.pdf/summary" {
		t.Errorf("path = %q", gotPath)
	}
	if s.Summary == "" {
		t.Error("expected non-empty summary")
	}
}

// ---- Compare ---------------------------------------------------------------

func TestCompare_SendsBodyAndReturnsComparison(t *testing.T) {
	var gotBody map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&gotBody)
		json.NewEncoder(w).Encode(rag.Comparison{
			File1:      "v1.pdf",
			File2:      "v2.pdf",
			Comparison: "• Clause 3 changed\n• Section 5 removed",
		})
	}))
	defer srv.Close()

	cmp, err := newTestClient(srv.URL).Compare(context.Background(), "policies", "v1.pdf", "v2.pdf")
	if err != nil {
		t.Fatalf("Compare() error: %v", err)
	}
	if gotBody["collection_name"] != "policies" || gotBody["file_1"] != "v1.pdf" || gotBody["file_2"] != "v2.pdf" {
		t.Errorf("unexpected request body: %v", gotBody)
	}
	if cmp.Comparison == "" {
		t.Error("expected non-empty comparison")
	}
}

func TestCompare_EmptyArgs_Error(t *testing.T) {
	c := newTestClient("http://localhost")
	if _, err := c.Compare(context.Background(), "", "a.pdf", "b.pdf"); err == nil {
		t.Error("expected error for empty collectionName")
	}
	if _, err := c.Compare(context.Background(), "col", "", "b.pdf"); err == nil {
		t.Error("expected error for empty file1")
	}
}
