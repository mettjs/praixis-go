package rag

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/mettjs/praixis-go/internal"
)

// Client sends requests to the /rag-db endpoints.
// Construct it via the top-level praixis.Client, not directly.
// Client is safe for concurrent use after construction.
type Client struct {
	cfg     internal.Config
	headers map[string]string
}

// New returns a Client. apiKey is sent as X-API-Key on every request.
func New(cfg internal.Config, apiKey string) *Client {
	return &Client{
		cfg:     cfg,
		headers: map[string]string{"X-API-Key": apiKey},
	}
}

// Ask opens a streaming RAG question request. The returned AskStream must be
// closed by the caller. SessionID, SearchQuery, and Sources are available
// immediately after this call returns, before the first Next() call.
func (c *Client) Ask(ctx context.Context, req QuestionRequest) (*AskStream, error) {
	body, err := internal.DoStream(ctx, c.cfg, http.MethodPost, "/rag-db/ask", c.headers, req)
	if err != nil {
		return nil, err
	}
	return newAskStream(internal.NewStreamReader(body)), nil
}

// Upload sends one or more files to a vector collection. The server processes
// each file independently; check UploadResponse.Results for per-file status.
// opts may be nil to use server defaults (collection "main", semantic chunking, chunk size 2000).
func (c *Client) Upload(ctx context.Context, files []FileUpload, opts *UploadOptions) (*UploadResponse, error) {
	if len(files) == 0 {
		return nil, fmt.Errorf("rag: at least one file is required")
	}

	fields := make(map[string]string)
	if opts != nil && opts.CollectionName != "" {
		fields["collection_name"] = opts.CollectionName
	}
	if opts != nil && opts.ChunkingStrategy != "" {
		fields["chunking_strategy"] = opts.ChunkingStrategy
	}
	if opts != nil && opts.ChunkSize != nil {
		fields["chunk_size"] = strconv.Itoa(*opts.ChunkSize)
	}
	if opts != nil && opts.ChunkOverlap != nil {
		fields["chunk_overlap"] = strconv.Itoa(*opts.ChunkOverlap)
	}
	if opts != nil && opts.ImprovedSearch {
		fields["improved_search"] = "true"
	}

	attachments := make([]internal.FileAttachment, len(files))
	for i, f := range files {
		attachments[i] = internal.FileAttachment{
			FieldName:   "files",
			Filename:    f.Filename,
			ContentType: f.ContentType,
			Data:        f.Data,
		}
	}

	var resp UploadResponse
	if err := internal.DoMultipart(ctx, c.cfg, "/rag-db/upload", c.headers, fields, attachments, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// Embed returns the 384-dimensional embedding vector for text.
// No LLM call is made; this uses the collection's embedding model directly.
func (c *Client) Embed(ctx context.Context, text string) (*EmbedResponse, error) {
	var resp EmbedResponse
	err := internal.DoJSON(ctx, c.cfg, http.MethodPost, "/rag-db/embed", c.headers,
		embedRequest{Text: text}, &resp)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

// ListCollections returns all vector collections owned by this API key's app.
func (c *Client) ListCollections(ctx context.Context) (*CollectionList, error) {
	var resp struct {
		TotalDocuments    int      `json:"total_documents"`
		ActiveCollections []string `json:"active_collections"`
	}
	if err := internal.DoJSON(ctx, c.cfg, http.MethodGet, "/rag-db/list", c.headers, nil, &resp); err != nil {
		return nil, err
	}
	return &CollectionList{
		TotalDocuments:    resp.TotalDocuments,
		ActiveCollections: resp.ActiveCollections,
	}, nil
}

// ListFiles returns the filenames stored in a collection.
func (c *Client) ListFiles(ctx context.Context, collectionName string) (*FileList, error) {
	if collectionName == "" {
		return nil, fmt.Errorf("rag: collectionName must not be empty")
	}
	var resp FileList
	path := "/rag-db/" + url.PathEscape(collectionName) + "/files"
	if err := internal.DoJSON(ctx, c.cfg, http.MethodGet, path, c.headers, nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// DeleteCollection permanently deletes a collection and all its chunks.
func (c *Client) DeleteCollection(ctx context.Context, collectionName string) error {
	if collectionName == "" {
		return fmt.Errorf("rag: collectionName must not be empty")
	}
	path := "/rag-db/delete/" + url.PathEscape(collectionName)
	return internal.DoJSON(ctx, c.cfg, http.MethodDelete, path, c.headers, nil, nil)
}

// DeleteFile permanently removes all chunks for a single file from a collection.
func (c *Client) DeleteFile(ctx context.Context, collectionName, filename string) error {
	if collectionName == "" {
		return fmt.Errorf("rag: collectionName must not be empty")
	}
	if filename == "" {
		return fmt.Errorf("rag: filename must not be empty")
	}
	path := "/rag-db/" + url.PathEscape(collectionName) + "/files/" + url.PathEscape(filename)
	return internal.DoJSON(ctx, c.cfg, http.MethodDelete, path, c.headers, nil, nil)
}

// Summarize returns a 3-sentence LLM-generated summary of a file in a collection.
func (c *Client) Summarize(ctx context.Context, collectionName, filename string) (*Summary, error) {
	if collectionName == "" {
		return nil, fmt.Errorf("rag: collectionName must not be empty")
	}
	if filename == "" {
		return nil, fmt.Errorf("rag: filename must not be empty")
	}
	path := "/rag-db/knowledge_base/" + url.PathEscape(collectionName) +
		"/files/" + url.PathEscape(filename) + "/summary"
	var resp Summary
	if err := internal.DoJSON(ctx, c.cfg, http.MethodGet, path, c.headers, nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// Compare returns a bullet-point diff between two files in the same collection.
func (c *Client) Compare(ctx context.Context, collectionName, file1, file2 string) (*Comparison, error) {
	if collectionName == "" {
		return nil, fmt.Errorf("rag: collectionName must not be empty")
	}
	if file1 == "" || file2 == "" {
		return nil, fmt.Errorf("rag: file1 and file2 must not be empty")
	}
	var resp Comparison
	err := internal.DoJSON(ctx, c.cfg, http.MethodPost, "/rag-db/knowledge_base/compare", c.headers,
		compareRequest{CollectionName: collectionName, File1: file1, File2: file2}, &resp)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}
