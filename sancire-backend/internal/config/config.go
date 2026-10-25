package config

import (
	"crypto/rand"
	"encoding/base64"
	"os"
	"strconv"
	"time"
)

type Config struct {
	Addr              string
	DataDir           string
	DBPath            string
	TemplatesDir      string
	CorpusDir         string
	QdrantURL         string
	QdrantCollection  string
	OllamaURL         string
	LLMModel          string
	EmbedModel        string
	MinRelevanceScore float64
	JWTSecret         string
	TokenTTL          time.Duration
}

func Load() Config {
	addr := getEnv("SANCIRE_ADDR", ":8080")
	dataDir := getEnv("SANCIRE_DATA_DIR", "./data")
	dbPath := getEnv("SANCIRE_DB_PATH", "./data/sancire.db")
	templatesDir := getEnv("SANCIRE_TEMPLATES_DIR", "./templates")
	corpusDir := getEnv("SANCIRE_CORPUS_DIR", "./corpus")
	qdrantURL := getEnv("SANCIRE_QDRANT_URL", "http://localhost:6333")
	collection := getEnv("SANCIRE_QDRANT_COLLECTION", "dou_knowledge")
	ollamaURL := getEnv("SANCIRE_OLLAMA_URL", "http://localhost:11434")
	llmModel := getEnv("SANCIRE_LLM_MODEL", "qwen3.5:4b")
	embedModel := getEnv("SANCIRE_EMBED_MODEL", "qwen3-embedding:0.6b")
	minScoreStr := getEnv("SANCIRE_MIN_SCORE", "0.45")
	minScore, err := strconv.ParseFloat(minScoreStr, 64)
	if err != nil {
		minScore = 0.45
	}
	secret := os.Getenv("SANCIRE_JWT_SECRET")
	if secret == "" {
		// Never ship a predictable signing key. Without an explicit deployment key,
		// tokens are intentionally process-local and rotate on restart.
		key := make([]byte, 32)
		if _, err := rand.Read(key); err != nil {
			panic("unable to generate a secure JWT signing key")
		}
		secret = base64.RawURLEncoding.EncodeToString(key)
	}
	return Config{
		Addr:              addr,
		DataDir:           dataDir,
		DBPath:            dbPath,
		TemplatesDir:      templatesDir,
		CorpusDir:         corpusDir,
		QdrantURL:         qdrantURL,
		QdrantCollection:  collection,
		OllamaURL:         ollamaURL,
		LLMModel:          llmModel,
		EmbedModel:        embedModel,
		MinRelevanceScore: minScore,
		JWTSecret:         secret,
		TokenTTL:          24 * time.Hour,
	}
}
func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}
