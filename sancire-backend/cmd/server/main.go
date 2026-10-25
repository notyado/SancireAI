package main
import (
	"context"
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
func main() {
	// Setup structured console logger
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339})
	cfg := config.Load()
	log.Info().Str("addr", cfg.Addr).Msg("Starting SancireAI Backend Service...")
	// Initialize SQLite Store
	dbStore, err := store.New(cfg.DBPath)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize database store")
	}
	defer dbStore.Close()
	log.Info().Str("db", cfg.DBPath).Msg("SQLite Store initialized")
	// Initialize RAG Engine
	ragEngine := rag.NewEngine(rag.EngineConfig{
		QdrantURL:         cfg.QdrantURL,
		Collection:        cfg.QdrantCollection,
		OllamaURL:         cfg.OllamaURL,
		LLMModel:          cfg.LLMModel,
		EmbedModel:        cfg.EmbedModel,
		MinRelevanceScore: cfg.MinRelevanceScore,
	})
	log.Info().Str("qdrant", cfg.QdrantURL).Str("ollama", cfg.OllamaURL).Msg("RAG Engine ready")
	// Setup Router & Handlers
	handler := api.NewHandler(cfg, dbStore, ragEngine)
	router := api.SetupRouter(handler)
	server := &http.Server{
		Addr:         cfg.Addr,
		Handler:      router,
		ReadTimeout:  300 * time.Second,
		WriteTimeout: 600 * time.Second,
		IdleTimeout:  300 * time.Second,
	}
	// Graceful shutdown channel
	stopChan := make(chan os.Signal, 1)
	signal.Notify(stopChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("Server failed to listen")
		}
	}()
	log.Info().Str("addr", cfg.Addr).Msg("SancireAI Backend listening successfully")
	<-stopChan
	log.Info().Msg("Shutting down SancireAI server...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		log.Error().Err(err).Msg("Server shutdown error")
	}
	log.Info().Msg("Server stopped gracefully")
}
