// Command qdrantload loads the local corpus (corpus/all_chunks.json + the
// clause chunks derived from documents.json) into the Qdrant collection used
// by the RAG engine. Idempotent: point ids are deterministic, so re-running
// overwrites in place.
//
// Usage:
//
//	go run ./cmd/qdrantload -limit 0
//
// Environment (same as the server): SANCIRE_QDRANT_URL, SANCIRE_QDRANT_COLLECTION,
// SANCIRE_OLLAMA_URL, SANCIRE_EMBED_MODEL.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"sancireai/backend/internal/config"
	"sancireai/backend/internal/kb"
	"sancireai/backend/internal/rag"
)

func main() {
	limit := flag.Int("limit", 0, "остановиться после N чанков (0 — всё)")
	corpus := flag.String("corpus", "", "путь к каталогу corpus (по умолчанию SANCIRE_CORPUS_DIR или ./corpus)")
	flag.Parse()

	// Ctrl-C cancels the embedding run instead of leaving half-pushed points.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg := config.Load()
	corpusDir := *corpus
	if corpusDir == "" {
		corpusDir = cfg.CorpusDir
	}
	ix, err := kb.Load(corpusDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ошибка чтения корпуса: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Корпус: %d фрагментов из %s\n", ix.Len(), corpusDir)

	eng := rag.NewEngine(rag.EngineConfig{
		QdrantURL:         cfg.QdrantURL,
		Collection:        cfg.QdrantCollection,
		OllamaURL:         cfg.OllamaURL,
		LLMModel:          cfg.LLMModel,
		EmbedModel:        cfg.EmbedModel,
		MinRelevanceScore: cfg.MinRelevanceScore,
	})

	if !eng.VectorHealthy(ctx) {
		fmt.Fprintf(os.Stderr, "Qdrant недоступен по адресу %s — запустите контейнер и повторите\n", cfg.QdrantURL)
		os.Exit(2)
	}
	if !eng.LLMHealthy(ctx) {
		fmt.Fprintf(os.Stderr, "Ollama недоступна по адресу %s — эмбеддинги не получить, запустите контейнер\n", cfg.OllamaURL)
		os.Exit(3)
	}
	fmt.Printf("Индексация в %s / %s (модель эмбеддингов: %s)…\n", cfg.QdrantURL, cfg.QdrantCollection, cfg.EmbedModel)
	start := time.Now()
	n, err := eng.LoadCorpus(ctx, ix, *limit)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ошибка загрузки: %v\n", err)
		os.Exit(4)
	}
	fmt.Printf("Готово: %d чанков за %s. Коллекция готова к поиску.\n", n, time.Since(start).Round(time.Second))
}
