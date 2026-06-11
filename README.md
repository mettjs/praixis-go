# praixis-go

Go SDK for **PraixisEngine** — a FastAPI backend for local LLM chat sessions and RAG (retrieval-augmented generation) over your own documents.

**Zero external dependencies.** Stdlib only.

---

## Installation

```bash
go get github.com/mettjs/praixis-go
```

Requires Go 1.22+.

---

## Quick start

```go
import (
    praixis "github.com/mettjs/praixis-go"
    "github.com/mettjs/praixis-go/chat"
    "github.com/mettjs/praixis-go/rag"
)

client := praixis.New("http://localhost:8080", "praixis_your_key")
```

The client exposes two sub-clients:

| Field | Package | Purpose |
|---|---|---|
| `client.Chat` | `chat` | Chat sessions, file summarization, session history |
| `client.RAG`  | `rag`  | Vector collections, document Q&A, upload, embed |

---

## Chat

### Streaming chat

```go
stream, err := client.Chat.Stream(ctx, chat.Request{
    Prompt: "Explain Go interfaces in two sentences.",
})
if err != nil { /* handle */ }
defer stream.Close()

fmt.Println(stream.SessionID()) // assigned by the server

for stream.Next() {
    fmt.Print(stream.Token())
}
if err := stream.Err(); err != nil { /* handle */ }
```

Continue an existing session by passing `SessionID`:

```go
stream, err := client.Chat.Stream(ctx, chat.Request{
    Prompt:    "Follow-up question",
    SessionID: previousSessionID,
})
```

### Summarize a file (streaming)

```go
f, _ := os.Open("report.pdf")
defer f.Close()

stream, err := client.Chat.SummarizeFile(ctx, "report.pdf", f, nil)
// or with options:
stream, err := client.Chat.SummarizeFile(ctx, "report.pdf", f,
    &chat.FileSummaryOptions{Task: "Extract action items", Tone: "Bullet points"},
)
defer stream.Close()

fmt.Println(stream.Filename())  // "report.pdf"
fmt.Println(stream.Progress())  // e.g. "reducing 4 chunks"
for stream.Next() {
    fmt.Print(stream.Token())
}
if stream.BackendError() != "" {
    // server-side error (e.g. GPU busy) reported inside the stream
}
```

### Session management

```go
// Fetch message history
history, err := client.Chat.History(ctx, sessionID)
for _, msg := range history.History {
    fmt.Printf("[%s] %s\n", msg.Role, msg.Content)
}

// List all active sessions
sessions, err := client.Chat.ActiveSessions(ctx)

// Delete a session
err = client.Chat.DeleteSession(ctx, sessionID)
```

---

## RAG

### Ask a question (streaming)

```go
stream, err := client.RAG.Ask(ctx, rag.QuestionRequest{
    CollectionName: "hr-docs",
    Question:       "How many vacation days do employees get?",
})
if err != nil { /* handle */ }
defer stream.Close()

fmt.Println(stream.SessionID())   // session for follow-up questions
fmt.Println(stream.SearchQuery()) // reformulated query used for retrieval

for stream.Next() {
    fmt.Print(stream.Token())
}
if err := stream.Err(); err != nil { /* handle */ }

fmt.Println(stream.Sources()) // []string{"policy.pdf", "hr-handbook.docx"}
```

### Upload documents

```go
f, _ := os.Open("policy.pdf")
defer f.Close()

resp, err := client.RAG.Upload(ctx,
    []rag.FileUpload{{Filename: "policy.pdf", Data: f}},
    &rag.UploadOptions{
        CollectionName:   "hr-docs",
        ChunkingStrategy: "semantic", // or "character" for fixed-size splits
        ChunkSize:        praixis.Ptr(2000),
        ChunkOverlap:     praixis.Ptr(150), // only used when ChunkingStrategy is "character"
        ImprovedSearch:   true,             // background hypothetical-question indexing for better natural-language search
    },
)
// resp.Succeeded, resp.Results[i].Status
```

Pass `nil` for `UploadOptions` to use server defaults (collection `"main"`, semantic chunking, chunk size 2000).

`Filename` is required — it is the document's stored identity and the server's primary format signal, so prefer a `.pdf`/`.docx`/`.txt` extension. For extension-less names the server falls back to the part's Content-Type (filled from the extension when `ContentType` is empty), then to the file's magic bytes.

`ImprovedSearch` enables hypothetical-question indexing: questions are generated in the background after the upload returns, so the document is searchable immediately and natural-language matching improves once generation finishes.

### Collections

```go
// List all collections
list, err := client.RAG.ListCollections(ctx)
// list.ActiveCollections []string, list.TotalDocuments int

// Files in a collection
files, err := client.RAG.ListFiles(ctx, "hr-docs")

// Delete a file from a collection
err = client.RAG.DeleteFile(ctx, "hr-docs", "old-policy.pdf")

// Delete an entire collection
err = client.RAG.DeleteCollection(ctx, "hr-docs")
```

### Summarize / compare files

```go
// 3-sentence LLM summary of a stored file
summary, err := client.RAG.Summarize(ctx, "hr-docs", "policy.pdf")
fmt.Println(summary.Summary)

// Bullet-point diff between two files
cmp, err := client.RAG.Compare(ctx, "hr-docs", "policy-v1.pdf", "policy-v2.pdf")
fmt.Println(cmp.Comparison)
```

### Embedding

```go
resp, err := client.RAG.Embed(ctx, "some text to embed")
// resp.Embedding []float64 (384 dimensions), resp.Dimensions int
```

---

## Error handling

```go
_, err := client.Chat.Stream(ctx, chat.Request{Prompt: "hi"})

switch {
case praixis.IsGPUBusy(err):
    // 503 — server is processing another request; retry after a moment
case praixis.IsRateLimit(err):
    // 429 — slow down
case praixis.IsUnauthorized(err):
    // 401/403 — check your API key
case praixis.IsNotFound(err):
    // 404 — collection, file, or session does not exist
}
```

The helpers live on the top-level `praixis` package. For full detail, extract the typed error with `errors.As`:

```go
var apiErr *praixis.APIError
if errors.As(err, &apiErr) {
    fmt.Println(apiErr.StatusCode, apiErr.Message)
}
```

---

## Configuration

```go
client := praixis.New(baseURL, apiKey,
    praixis.WithTimeout(60*time.Second), // default 30s; does not affect streams
)
```

Streams are not subject to the HTTP timeout — cancel them via `context.Context`:

```go
ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
defer cancel()
stream, err := client.Chat.Stream(ctx, ...)
```

---

## Examples

Runnable examples are in `_examples/`:

| Directory | Description |
|---|---|
| `_examples/chat_stream` | Stream a single chat turn |
| `_examples/rag_ask`     | Ask a question over a collection |
| `_examples/rag_upload`  | Upload a file into a collection |

```bash
PRAIXIS_URL=http://localhost:8080 PRAIXIS_API_KEY=praixis_xxx go run ./_examples/chat_stream
```

---

## Package layout

```
praixis-go/
├── praixis.go          # New(), Ptr(), error helpers (APIError, IsGPUBusy, …), Option types
├── chat/
│   ├── types.go        # Request, History, FileSummaryOptions, …
│   ├── stream.go       # ChatStream, FileSummaryStream
│   └── client.go       # Stream, SummarizeFile, History, ActiveSessions, DeleteSession
├── rag/
│   ├── types.go        # QuestionRequest, FileUpload, UploadOptions, …
│   ├── stream.go       # AskStream
│   └── client.go       # Ask, Upload, Embed, ListCollections, …
└── internal/
    ├── httpclient.go   # DoJSON, DoStream, DoMultipart, APIError, …
    └── stream.go       # StreamReader (metadata + token iterator)
```

---

## License

MIT — see [LICENSE](LICENSE).
