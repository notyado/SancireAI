// Command server starts the SancireAI HTTP service: SQLite store, RAG engine
// (Qdrant + Ollama) and the API router. Shutdown is graceful on SIGINT/SIGTERM.
package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"sancireai/backend/internal/api"
	"sancireai/backend/internal/config"
	"sancireai/backend/internal/rag"
	"sancireai/backend/internal/store"
)

// Server timeouts. Reads and writes are generous because document generation
// and SSE answers are long-lived by design.
const (
	readTimeout     = 300 * time.Second
	writeTimeout    = 600 * time.Second
	idleTimeout     = 300 * time.Second
	shutdownTimeout = 10 * time.Second
)

func main() {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339})

	cfg := config.Load()

	dbStore, err := store.New(cfg.DBPath)
	if err != nil {
		log.Fatal().Err(err).Str("db", cfg.DBPath).Msg("failed to initialize the database store")
	}
	defer func() {
		if err := dbStore.Close(); err != nil {
			log.Error().Err(err).Msg("database close failed")
		}
	}()

	ragEngine := rag.NewEngine(rag.EngineConfig{
		QdrantURL:         cfg.QdrantURL,
		Collection:        cfg.QdrantCollection,
		OllamaURL:         cfg.OllamaURL,
		LLMModel:          cfg.LLMModel,
		EmbedModel:        cfg.EmbedModel,
		MinRelevanceScore: cfg.MinRelevanceScore,
		EmbedTimeout:      cfg.EmbedTimeout,
	})

	server := &http.Server{
		Addr:         cfg.Addr,
		Handler:      api.SetupRouter(api.NewHandler(cfg, dbStore, ragEngine)),
		ReadTimeout:  readTimeout,
		WriteTimeout: writeTimeout,
		IdleTimeout:  idleTimeout,
	}

	// A listen error must not be reported with log.Fatal from the serving
	// goroutine: it would skip the deferred cleanup and race the shutdown path.
	errChan := make(chan error, 1)
	go func() {
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errChan <- err
		}
	}()

	stopChan := make(chan os.Signal, 1)
	signal.Notify(stopChan, os.Interrupt, syscall.SIGTERM)

	log.Info().
		Str("addr", cfg.Addr).
		Str("db", cfg.DBPath).
		Str("qdrant", cfg.QdrantURL).
		Str("ollama", cfg.OllamaURL).
		Msg("SancireAI backend is listening")

	select {
	case err := <-errChan:
		log.Fatal().Err(err).Msg("server failed to listen")
	case <-stopChan:
		log.Info().Msg("shutdown signal received")
	}

	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		log.Error().Err(err).Msg("server shutdown error")
	}
	log.Info().Msg("server stopped gracefully")
}
