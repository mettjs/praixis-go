package internal

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"path/filepath"
	"strings"
	"time"
)

// Config holds the shared HTTP settings used by all resource clients.
type Config struct {
	BaseURL    string
	HTTPClient *http.Client
}

// NewConfig returns a Config with a default-timeout HTTP client.
// baseURL should not have a trailing slash.
func NewConfig(baseURL string, timeout time.Duration) Config {
	return Config{
		BaseURL:    strings.TrimRight(baseURL, "/"),
		HTTPClient: &http.Client{Timeout: timeout},
	}
}

// APIError is returned for any non-2xx HTTP response.
type APIError struct {
	StatusCode int
	Message    string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("praixis: HTTP %d: %s", e.StatusCode, e.Message)
}

// IsRateLimit reports whether the error is a 429 Too Many Requests.
func IsRateLimit(err error) bool {
	var ae *APIError
	return errors.As(err, &ae) && ae.StatusCode == http.StatusTooManyRequests
}

// IsUnauthorized reports whether the error is a 401 or 403.
func IsUnauthorized(err error) bool {
	var ae *APIError
	return errors.As(err, &ae) &&
		(ae.StatusCode == http.StatusUnauthorized || ae.StatusCode == http.StatusForbidden)
}

// IsGPUBusy reports whether the error is a 503 GPU slots exhausted response.
func IsGPUBusy(err error) bool {
	var ae *APIError
	return errors.As(err, &ae) && ae.StatusCode == http.StatusServiceUnavailable
}

// DoJSON sends a JSON request and decodes the response body into dst.
// Pass nil body for GET/DELETE with no request body.
// Pass nil dst to discard the response body.
func DoJSON(ctx context.Context, cfg Config, method, path string, headers map[string]string, body, dst any) error {
	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("praixis: marshal request: %w", err)
		}
		bodyReader = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, cfg.BaseURL+path, bodyReader)
	if err != nil {
		return fmt.Errorf("praixis: build request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	applyHeaders(req, headers)

	resp, err := cfg.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("praixis: do request: %w", err)
	}
	defer resp.Body.Close()

	if !is2xx(resp.StatusCode) {
		return readAPIError(resp)
	}
	if dst == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(dst); err != nil {
		return fmt.Errorf("praixis: decode response: %w", err)
	}
	return nil
}

// DoStream sends a JSON request and returns the raw response body for streaming.
// The client's Timeout is deliberately not applied to the body read so the
// stream is only bounded by ctx. The caller must close the returned body
// (StreamReader.Close handles this).
func DoStream(ctx context.Context, cfg Config, method, path string, headers map[string]string, body any) (io.ReadCloser, error) {
	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("praixis: marshal request: %w", err)
		}
		bodyReader = bytes.NewReader(b)
	}

	// Strip the global timeout so the connection stays open during streaming.
	streamClient := &http.Client{Transport: cfg.HTTPClient.Transport}

	req, err := http.NewRequestWithContext(ctx, method, cfg.BaseURL+path, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("praixis: build request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "text/event-stream")
	applyHeaders(req, headers)

	resp, err := streamClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("praixis: do request: %w", err)
	}
	if !is2xx(resp.StatusCode) {
		defer resp.Body.Close()
		return nil, readAPIError(resp)
	}
	return resp.Body, nil
}

// FileAttachment represents a file to include in a multipart/form-data request.
type FileAttachment struct {
	FieldName   string    // form field name (e.g. "file" or "files")
	Filename    string    // original filename, used for Content-Disposition
	ContentType string    // MIME type; left empty to auto-detect from extension
	Data        io.Reader // file content
}

// DoMultipart sends a multipart/form-data POST and decodes the JSON response into dst.
func DoMultipart(ctx context.Context, cfg Config, path string, headers map[string]string, fields map[string]string, files []FileAttachment, dst any) error {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)

	for k, v := range fields {
		if err := mw.WriteField(k, v); err != nil {
			return fmt.Errorf("praixis: write form field %q: %w", k, err)
		}
	}

	for _, f := range files {
		ct := f.ContentType
		if ct == "" {
			ct = mimeByExtension(f.Filename)
		}
		h := make(textproto.MIMEHeader)
		h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, f.FieldName, f.Filename))
		h.Set("Content-Type", ct)
		w, err := mw.CreatePart(h)
		if err != nil {
			return fmt.Errorf("praixis: create multipart part for %q: %w", f.Filename, err)
		}
		if _, err := io.Copy(w, f.Data); err != nil {
			return fmt.Errorf("praixis: write file %q: %w", f.Filename, err)
		}
	}
	mw.Close()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.BaseURL+path, &buf)
	if err != nil {
		return fmt.Errorf("praixis: build request: %w", err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	applyHeaders(req, headers)

	resp, err := cfg.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("praixis: do request: %w", err)
	}
	defer resp.Body.Close()

	if !is2xx(resp.StatusCode) {
		return readAPIError(resp)
	}
	if dst == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(dst); err != nil {
		return fmt.Errorf("praixis: decode response: %w", err)
	}
	return nil
}

// DoStreamMultipart sends a multipart/form-data POST and returns the raw response
// body for streaming. Used by the file_summary endpoint.
func DoStreamMultipart(ctx context.Context, cfg Config, path string, headers map[string]string, fields map[string]string, files []FileAttachment) (io.ReadCloser, error) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)

	for k, v := range fields {
		if err := mw.WriteField(k, v); err != nil {
			return nil, fmt.Errorf("praixis: write form field %q: %w", k, err)
		}
	}

	for _, f := range files {
		ct := f.ContentType
		if ct == "" {
			ct = mimeByExtension(f.Filename)
		}
		h := make(textproto.MIMEHeader)
		h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, f.FieldName, f.Filename))
		h.Set("Content-Type", ct)
		w, err := mw.CreatePart(h)
		if err != nil {
			return nil, fmt.Errorf("praixis: create multipart part for %q: %w", f.Filename, err)
		}
		if _, err := io.Copy(w, f.Data); err != nil {
			return nil, fmt.Errorf("praixis: write file %q: %w", f.Filename, err)
		}
	}
	mw.Close()

	streamClient := &http.Client{Transport: cfg.HTTPClient.Transport}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.BaseURL+path, &buf)
	if err != nil {
		return nil, fmt.Errorf("praixis: build request: %w", err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Accept", "text/event-stream")
	applyHeaders(req, headers)

	resp, err := streamClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("praixis: do request: %w", err)
	}
	if !is2xx(resp.StatusCode) {
		defer resp.Body.Close()
		return nil, readAPIError(resp)
	}
	return resp.Body, nil
}

func applyHeaders(req *http.Request, headers map[string]string) {
	for k, v := range headers {
		req.Header.Set(k, v)
	}
}

func is2xx(code int) bool {
	return code >= 200 && code < 300
}

// readAPIError reads the response body and constructs an APIError.
// FastAPI encodes errors as {"detail": "..."} or {"detail": [...]}.
func readAPIError(resp *http.Response) error {
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	msg := strings.TrimSpace(string(raw))

	var envelope struct {
		Detail any `json:"detail"`
	}
	if json.Unmarshal(raw, &envelope) == nil && envelope.Detail != nil {
		switch v := envelope.Detail.(type) {
		case string:
			msg = v
		default:
			if b, err := json.Marshal(v); err == nil {
				msg = string(b)
			}
		}
	}
	return &APIError{StatusCode: resp.StatusCode, Message: msg}
}

func mimeByExtension(filename string) string {
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".pdf":
		return "application/pdf"
	case ".docx":
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	case ".txt":
		return "text/plain"
	default:
		return "application/octet-stream"
	}
}
