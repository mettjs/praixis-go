// chat_stream demonstrates streaming a single-turn chat request.
//
// Usage:
//
//	go run ./_examples/chat_stream
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/mettjs/praixis-go/chat"

	praixis "github.com/mettjs/praixis-go"
)

func main() {
	baseURL := envOr("PRAIXIS_URL", "http://localhost:8080")
	apiKey := envOr("PRAIXIS_API_KEY", "praixis_your_key")

	client := praixis.New(baseURL, apiKey)

	stream, err := client.Chat.Stream(context.Background(), chat.Request{
		Prompt: "Explain Go interfaces in two sentences.",
	})
	if err != nil {
		log.Fatalf("Stream: %v", err)
	}
	defer stream.Close()

	fmt.Printf("[session: %s]\n", stream.SessionID())

	for stream.Next() {
		fmt.Print(stream.Token())
	}
	fmt.Println()

	if err := stream.Err(); err != nil {
		log.Fatalf("stream error: %v", err)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
