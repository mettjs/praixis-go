// Package praixis is the top-level entry point for the Praixis Go SDK.
//
// Create a client with your server URL and API key, then access the Chat and
// RAG sub-clients directly:
//
//	client := praixis.New("http://localhost:8080", "praixis_your_key")
//
//	// streaming chat
//	stream, err := client.Chat.Stream(ctx, chat.Request{Prompt: "Hello"})
//
//	// RAG question
//	stream, err := client.RAG.Ask(ctx, rag.QuestionRequest{
//	    CollectionName: "docs",
//	    Question:       "What is the refund policy?",
//	})
package praixis

import (
	"time"

	"github.com/mettjs/praixis-go/chat"
	"github.com/mettjs/praixis-go/internal"
	"github.com/mettjs/praixis-go/rag"
)

// DefaultTimeout is the HTTP request timeout applied to non-streaming requests.
// Streaming endpoints are not subject to this timeout; use context.Context instead.
const DefaultTimeout = 30 * time.Second

// Ptr returns a pointer to v. It is a convenience for setting optional pointer
// fields such as rag.UploadOptions.ChunkSize, since Go does not allow taking the
// address of a literal (e.g. &800 is invalid).
//
//	opts := &rag.UploadOptions{ChunkSize: praixis.Ptr(800)}
func Ptr[T any](v T) *T { return &v }

// APIError is returned for any non-2xx HTTP response. It carries the HTTP status
// code and the server's error message. Use errors.As to extract it, or the
// IsGPUBusy / IsRateLimit / IsUnauthorized helpers to test for common cases.
type APIError = internal.APIError

// IsGPUBusy reports whether err is a 503 response, meaning the server's GPU
// slots are exhausted. Retry after a short delay.
func IsGPUBusy(err error) bool { return internal.IsGPUBusy(err) }

// IsRateLimit reports whether err is a 429 Too Many Requests response.
func IsRateLimit(err error) bool { return internal.IsRateLimit(err) }

// IsUnauthorized reports whether err is a 401 or 403 response, usually meaning
// the API key is missing or invalid.
func IsUnauthorized(err error) bool { return internal.IsUnauthorized(err) }

// Client is the root Praixis SDK client. Access capabilities through the Chat
// and RAG fields.
type Client struct {
	Chat *chat.Client
	RAG  *rag.Client
}

// Option configures the Client.
type Option func(*options)

type options struct {
	timeout time.Duration
}

// WithTimeout overrides the default HTTP request timeout (30 s).
// Streaming endpoints ignore this timeout — use context.Context for
// cancellation on streams.
func WithTimeout(d time.Duration) Option {
	return func(o *options) { o.timeout = d }
}

// New creates a Client pointed at baseURL, authenticating with apiKey.
// baseURL should not have a trailing slash (e.g. "http://localhost:8080").
func New(baseURL, apiKey string, opts ...Option) *Client {
	o := &options{timeout: DefaultTimeout}
	for _, opt := range opts {
		opt(o)
	}
	cfg := internal.NewConfig(baseURL, o.timeout)
	return &Client{
		Chat: chat.New(cfg, apiKey),
		RAG:  rag.New(cfg, apiKey),
	}
}
