// Package chat provides a client for the Praixis chat and file-summary endpoints.
//
// All streaming endpoints return a Stream type that must be closed by the caller:
//
//	stream, err := client.Stream(ctx, chat.Request{Prompt: "hello"})
//	if err != nil { ... }
//	defer stream.Close()
//
//	fmt.Println(stream.SessionID())
//	for stream.Next() {
//	    fmt.Print(stream.Token())
//	}
//	if err := stream.Err(); err != nil { ... }
package chat

// Request is the body for POST /general-requests/chat.
type Request struct {
	Prompt         string `json:"prompt"`
	SystemPrompt   string `json:"system_prompt,omitempty"`
	SessionID      string `json:"session_id,omitempty"`
	ResponseFormat string `json:"response_format,omitempty"` // "text" (default) | "json"
}

// Message is one entry in a conversation history.
type Message struct {
	Role    string `json:"role"` // "user" | "assistant" | "system"
	Content string `json:"content"`
}

// History is the response from GET /general-requests/chat/{session_id}.
type History struct {
	SessionID string    `json:"session_id"`
	History   []Message `json:"history"`
}

// FileSummaryOptions are the optional form fields for POST /general-requests/file_summary.
// Zero-value fields fall back to the server defaults.
type FileSummaryOptions struct {
	Task string // e.g. "Extract all action items". Default: "Summarize the key points."
	Tone string // e.g. "Casual". Default: "Professional and objective"
}
