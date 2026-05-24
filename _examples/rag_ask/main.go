// rag_ask demonstrates asking a question against a RAG collection.
//
// Usage:
//
//	go run ./_examples/rag_ask
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	praixis "github.com/mettjs/praixis-go"
	"github.com/mettjs/praixis-go/rag"
)

func main() {
	baseURL := envOr("PRAIXIS_URL", "http://localhost:8080")
	apiKey := envOr("PRAIXIS_API_KEY", "praixis_your_key")
	collection := envOr("PRAIXIS_COLLECTION", "main")

	client := praixis.New(baseURL, apiKey)

	stream, err := client.RAG.Ask(context.Background(), rag.QuestionRequest{
		CollectionName: collection,
		Question:       "Summarize the key points in the uploaded documents.",
	})
	if err != nil {
		log.Fatalf("Ask: %v", err)
	}
	defer stream.Close()

	fmt.Printf("[session: %s]\n", stream.SessionID())
	fmt.Printf("[query:   %s]\n", stream.SearchQuery())

	for stream.Next() {
		fmt.Print(stream.Token())
	}
	fmt.Println()

	if err := stream.Err(); err != nil {
		log.Fatalf("stream error: %v", err)
	}

	if sources := stream.Sources(); len(sources) > 0 {
		fmt.Printf("\nSources: %s\n", strings.Join(sources, ", "))
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
