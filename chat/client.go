package chat

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/mettjs/praixis-go/internal"
)

// Client sends requests to the /general-requests chat endpoints.
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

// Stream opens a streaming chat request. The returned ChatStream must be closed
// by the caller. SessionID() is available immediately after this call returns.
//
// Pass req.SessionID to continue an existing session; leave it empty to start a
// new one. The session ID assigned by the server is available via ChatStream.SessionID().
func (c *Client) Stream(ctx context.Context, req Request) (*ChatStream, error) {
	body, err := internal.DoStream(ctx, c.cfg, http.MethodPost, "/general-requests/chat", c.headers, req)
	if err != nil {
		return nil, err
	}
	return newChatStream(internal.NewStreamReader(body)), nil
}

// SummarizeFile uploads a file and streams the LLM-generated summary or analysis.
// filename is used for the Content-Disposition header and is echoed in FileSummaryStream.Filename().
// opts may be nil to use server defaults (task = "Summarize the key points.", tone = "Professional and objective").
func (c *Client) SummarizeFile(ctx context.Context, filename string, data io.Reader, opts *FileSummaryOptions) (*FileSummaryStream, error) {
	fields := make(map[string]string)
	if opts != nil {
		if opts.Task != "" {
			fields["task"] = opts.Task
		}
		if opts.Tone != "" {
			fields["tone"] = opts.Tone
		}
	}

	files := []internal.FileAttachment{
		{FieldName: "file", Filename: filename, Data: data},
	}

	body, err := internal.DoStreamMultipart(ctx, c.cfg, "/general-requests/file_summary", c.headers, fields, files)
	if err != nil {
		return nil, err
	}
	return newFileSummaryStream(internal.NewStreamReader(body)), nil
}

// History returns the full message history for a session.
// Returns a 404-wrapped APIError if the session does not exist or has expired.
func (c *Client) History(ctx context.Context, sessionID string) (*History, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("chat: sessionID must not be empty")
	}
	var h History
	err := internal.DoJSON(ctx, c.cfg, http.MethodGet,
		"/general-requests/chat/"+url.PathEscape(sessionID),
		c.headers, nil, &h,
	)
	if err != nil {
		return nil, err
	}
	return &h, nil
}

// ActiveSessions returns the IDs of all live sessions for this API key's app.
func (c *Client) ActiveSessions(ctx context.Context) ([]string, error) {
	var resp struct {
		ActiveSessions []string `json:"active_sessions"`
	}
	err := internal.DoJSON(ctx, c.cfg, http.MethodGet,
		"/general-requests/chat/sessions/active",
		c.headers, nil, &resp,
	)
	if err != nil {
		return nil, err
	}
	return resp.ActiveSessions, nil
}

// DeleteSession deletes a session and its message history.
// Returns a 404-wrapped APIError if the session does not exist.
func (c *Client) DeleteSession(ctx context.Context, sessionID string) error {
	if sessionID == "" {
		return fmt.Errorf("chat: sessionID must not be empty")
	}
	return internal.DoJSON(ctx, c.cfg, http.MethodDelete,
		"/general-requests/chat/"+url.PathEscape(sessionID),
		c.headers, nil, nil,
	)
}
