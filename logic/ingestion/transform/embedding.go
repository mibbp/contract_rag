package transform

import (
	"context"
	"github.com/cloudwego/eino-ext/components/embedding/ollama"
	"log"
	"os"
	"time"
)

func NewEmbedder(ctx context.Context, config ollama.EmbeddingConfig) (*ollama.Embedder, error) {
	//ctx := context.Background()

	baseURL := os.Getenv("OLLAMA_BASE_URL")
	if baseURL == "" {
		baseURL = "http://localhost:11434" // 默认本地
	}
	model := os.Getenv("OLLAMA_EMBED_MODEL")
	if model == "" {
		model = "bge-m3"
	}

	embedder, err := ollama.NewEmbedder(ctx, &ollama.EmbeddingConfig{
		BaseURL: baseURL,
		Model:   model,
		Timeout: 10 * time.Second,
	})
	if err != nil {
		log.Fatalf("NewEmbedder of ollama error: %v", err)
		return nil, err
	}

	log.Printf("===== call Embedder directly =====")

	vectors, err := embedder.EmbedStrings(ctx, []string{"hello", "how are you"})
	if err != nil {
		log.Fatalf("EmbedStrings of Ollama failed, err=%v", err)
	}

	log.Printf("vectors : %v", vectors)

	return embedder, nil
}
