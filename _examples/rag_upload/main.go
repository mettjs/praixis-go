// rag_upload demonstrates uploading a file into a RAG collection.
//
// Usage:
//
//	go run ./_examples/rag_upload path/to/file.pdf
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"

	praixis "github.com/mettjs/praixis-go"
	"github.com/mettjs/praixis-go/rag"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: rag_upload <file>")
		os.Exit(1)
	}

	baseURL := envOr("PRAIXIS_URL", "http://localhost:8080")
	apiKey := envOr("PRAIXIS_API_KEY", "praixis_your_key")
	collection := envOr("PRAIXIS_COLLECTION", "main")

	client := praixis.New(baseURL, apiKey)

	path := os.Args[1]
	f, err := os.Open(path)
	if err != nil {
		log.Fatalf("open file: %v", err)
	}
	defer f.Close()

	resp, err := client.RAG.Upload(context.Background(),
		[]rag.FileUpload{{
			Filename: filepath.Base(path),
			Data:     f,
		}},
		&rag.UploadOptions{CollectionName: collection},
	)
	if err != nil {
		log.Fatalf("Upload: %v", err)
	}

	fmt.Printf("Collection: %s\n", resp.CollectionName)
	fmt.Printf("Processed:  %d / %d succeeded\n", resp.Succeeded, resp.Processed)
	for _, r := range resp.Results {
		status := r.Status
		if r.Detail != "" {
			status += " — " + r.Detail
		}
		fmt.Printf("  %s: %s\n", r.Filename, status)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
