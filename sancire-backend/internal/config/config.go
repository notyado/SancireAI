// Package config resolves runtime settings from the environment. Every value
// has a sane local-stack default, so the service starts without any .env file.
package config

import (
	"crypto/rand"
	"encoding/base64"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
)

const (
	// DefaultMinRelevanceScore is the cosine-similarity floor for vector search.
	DefaultMinRelevanceScore = 0.45
	// DefaultTokenTTL is how long an issued JWT stays valid.
	DefaultTokenTTL = 24 * time.Hour
	// DefaultDBFile is the SQLite file name inside DataDir.
	DefaultDBFile = "sancire.db"
)

// DefaultCORSOrigins are the local development UIs allowed to call the API
// with credentials. Override with SANCIRE_CORS_ORIGINS (comma separated).
var DefaultCORSOrigins = []string{
	"http://localhost:3000",
	"http://127.0.0.1:3000",
	"http://localhost:5173",
	"http://127.0.0.1:5173",
}

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
	EmbedTimeout      time.Duration
	JWTSecret         string
	TokenTTL          time.Duration
	CORSOrigins       []string
}

func Load() Config {
	dataDir := getEnv("SANCIRE_DATA_DIR", "./data")
	minScore := getEnvFloat("SANCIRE_MIN_SCORE", DefaultMinRelevanceScore)
	tokenTTL := getEnvDuration("SANCIRE_TOKEN_TTL", DefaultTokenTTL)

	// Эмбеддинги могут считаться не на этой машине (например, на ноутбуке
	// за SSH-туннелем), поэтому таймаут одного запроса настраивается.
	embedTimeout := getEnvDuration("SANCIRE_EMBED_TIMEOUT", 10*time.Second)
	if raw := os.Getenv("SANCIRE_EMBED_TIMEOUT"); raw != "" {
		if sec, err := strconv.Atoi(raw); err == nil && sec > 0 {
			embedTimeout = time.Duration(sec) * time.Second
		}
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
		log.Warn().Msg("SANCIRE_JWT_SECRET is not set — generated an ephemeral key, sessions will not survive a restart")
	}

	return Config{
		Addr:              getEnv("SANCIRE_ADDR", ":8080"),
		DataDir:           dataDir,
		DBPath:            getEnv("SANCIRE_DB_PATH", filepath.Join(dataDir, DefaultDBFile)),
		TemplatesDir:      getEnv("SANCIRE_TEMPLATES_DIR", "./templates"),
		CorpusDir:         getEnv("SANCIRE_CORPUS_DIR", "./corpus"),
		QdrantURL:         getEnv("SANCIRE_QDRANT_URL", "http://localhost:6333"),
		QdrantCollection:  getEnv("SANCIRE_QDRANT_COLLECTION", "dou_knowledge"),
		OllamaURL:         getEnv("SANCIRE_OLLAMA_URL", "http://localhost:11434"),
		LLMModel:          getEnv("SANCIRE_LLM_MODEL", "qwen3.5:4b"),
		EmbedModel:        getEnv("SANCIRE_EMBED_MODEL", "qwen3-embed"),
		MinRelevanceScore: minScore,
		EmbedTimeout:      embedTimeout,
		JWTSecret:         secret,
		TokenTTL:          tokenTTL,
		CORSOrigins:       getEnvList("SANCIRE_CORS_ORIGINS", DefaultCORSOrigins),
	}
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func getEnvFloat(key string, defaultVal float64) float64 {
	raw := os.Getenv(key)
	if raw == "" {
		return defaultVal
	}
	val, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		log.Warn().Str("key", key).Str("value", raw).Float64("default", defaultVal).Msg("invalid float, using default")
		return defaultVal
	}
	return val
}

func getEnvDuration(key string, defaultVal time.Duration) time.Duration {
	raw := os.Getenv(key)
	if raw == "" {
		return defaultVal
	}
	val, err := time.ParseDuration(raw)
	if err != nil || val <= 0 {
		log.Warn().Str("key", key).Str("value", raw).Str("default", defaultVal.String()).Msg("invalid duration, using default")
		return defaultVal
	}
	return val
}

func getEnvList(key string, defaultVal []string) []string {
	raw := os.Getenv(key)
	if strings.TrimSpace(raw) == "" {
		return defaultVal
	}
	var out []string
	for _, item := range strings.Split(raw, ",") {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	if len(out) == 0 {
		return defaultVal
	}
	return out
}
