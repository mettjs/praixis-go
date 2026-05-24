// Package rag provides a client for the Praixis RAG (retrieval-augmented generation)
// and vector-store endpoints.
//
// Streaming ask requests follow the same pattern as chat:
//
//	stream, err := client.Ask(ctx, rag.QuestionRequest{...})
//	if err != nil { ... }
//	defer stream.Close()
//
//	fmt.Println(stream.SessionID(), stream.SearchQuery(), stream.Sources())
//	for stream.Next() {
//	    fmt.Print(stream.Token())
//	}
//	if err := stream.Err(); err != nil { ... }
package rag

import "io"

// QuestionRequest is the body for POST /rag-db/ask.
type QuestionRequest struct {
	CollectionName string         `json:"collection_name"`
	Question       string         `json:"question"`
	SessionID      string         `json:"session_id,omitempty"`
	NResults       int            `json:"n_results,omitempty"` // 1–20; server default 5
	SystemPrompt   string         `json:"system_prompt,omitempty"`
	MetadataFilter map[string]any `json:"metadata_filter,omitempty"` // ChromaDB where-filter
}

// FileUpload is one file to include in an Upload request.
type FileUpload struct {
	Filename    string    // original filename; used for Content-Disposition and stored in the collection
	ContentType string    // MIME type; leave empty to auto-detect from extension
	Data        io.Reader // file content
}

// UploadOptions are the optional form parameters for POST /rag-db/upload.
// Nil pointer fields default to server values (CollectionName → "main",
// ChunkSize → 1000, ChunkOverlap → 150).
type UploadOptions struct {
	CollectionName string // default "main"
	ChunkSize      *int   // 100–4000; nil → server default
	ChunkOverlap   *int   // 0–500;   nil → server default
}

// UploadResult is the per-file outcome returned by Upload.
type UploadResult struct {
	Filename string `json:"filename"`
	Status   string `json:"status"` // "success" | "error"
	Detail   string `json:"detail,omitempty"`
}

// UploadResponse is the full response from POST /rag-db/upload.
type UploadResponse struct {
	CollectionName string         `json:"collection_name"`
	Processed      int            `json:"processed"`
	Succeeded      int            `json:"succeeded"`
	Results        []UploadResult `json:"results"`
}

// CollectionList is the response from GET /rag-db/list.
type CollectionList struct {
	TotalDocuments    int      `json:"total_documents"`
	ActiveCollections []string `json:"active_collections"`
}

// FileList is the response from GET /rag-db/{collection_name}/files.
type FileList struct {
	CollectionName string   `json:"collection_name"`
	TotalFiles     int      `json:"total_files"`
	FilesStored    []string `json:"files_stored"`
}

// EmbedResponse is the response from POST /rag-db/embed.
type EmbedResponse struct {
	Text       string    `json:"text"`
	Dimensions int       `json:"dimensions"`
	Embedding  []float64 `json:"embedding"`
}

// Summary is the response from GET /rag-db/knowledge_base/{collection}/files/{file}/summary.
type Summary struct {
	Filename string `json:"filename"`
	Summary  string `json:"summary"`
}

// Comparison is the response from POST /rag-db/knowledge_base/compare.
type Comparison struct {
	File1      string `json:"file_1"`
	File2      string `json:"file_2"`
	Comparison string `json:"comparison"`
}

type embedRequest struct {
	Text string `json:"text"`
}

type compareRequest struct {
	CollectionName string `json:"collection_name"`
	File1          string `json:"file_1"`
	File2          string `json:"file_2"`
}
