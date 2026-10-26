package rag

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"sancireai/backend/internal/kb"
)

// Timing and batching constants of the RAG pipeline. Everything that talks to
// Ollama or Qdrant is bounded by one of these values.
const (
	httpClientTimeout = 240 * time.Second // full LLM generation
	probeTimeout      = 1500 * time.Millisecond
	probeCacheTTL     = 30 * time.Second
	embedTimeout      = 8 * time.Second
	searchTimeout     = 8 * time.Second
	generateTimeout   = 90 * time.Second
	explainTimeout    = 60 * time.Second
	pushChunkTimeout  = 10 * time.Second

	pointBatch          = 64  // Qdrant upsert batch
	corpusBatch         = 500 // chunks per LoadCorpus push
	streamRunes         = 24  // runes per SSE delta
	streamDelay         = 8 * time.Millisecond
	flushRunes          = 200 // buffered runes before the refusal gate opens
	maxHistoryMsgs      = 12  // prior conversation turns forwarded to the model
	maxHistoryRunes     = 4000
	explainSnippetRunes = 200

	vectorDistance = "Cosine" // metric of the Qdrant collection
)

// AttachedFile describes a chat attachment stored in message metadata.
type AttachedFile struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
	Type string `json:"type"`
}

// MessageMeta is the metadata of an LLM-backed answer. The rich card metadata
// (sources, process diagrams, documents) is produced by the dialog engine; the
// RAG engine only serves the «Объяснить» and «Эксперт» actions.
type MessageMeta struct {
	NotFound  bool     `json:"notFound,omitempty"`
	FollowUps []string `json:"followUps,omitempty"`
	Expert    bool     `json:"expert,omitempty"`
}

// ScoredChunk is one knowledge fragment with its similarity score. It is the
// exchange format between the local KB index, Qdrant and the dialog engine.
type ScoredChunk struct {
	Score         float64 `json:"score"`
	ChunkID       string  `json:"chunkId"`
	SectionID     string  `json:"sectionId"`
	DocumentTitle string  `json:"documentTitle"`
	Clause        string  `json:"clause"`
	Title         string  `json:"title"`
	Text          string  `json:"text"`
	Page          int     `json:"page"`
	UpdatedAt     string  `json:"updatedAt"`
	VersionID     string  `json:"versionId,omitempty"`
	Category      string  `json:"category"`
}

// ChatMessage is one turn of the conversation history forwarded to the model.
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// EngineConfig points the engine at the local Ollama and Qdrant instances.
type EngineConfig struct {
	QdrantURL         string
	Collection        string
	OllamaURL         string
	LLMModel          string
	EmbedModel        string
	MinRelevanceScore float64
}

// Engine talks to the local (on-premise) inference stack: Ollama for
// embeddings and generation, Qdrant for vector search.
type Engine struct {
	cfg        EngineConfig
	httpClient *http.Client
	probe      struct {
		mu       sync.Mutex
		llmOK    bool
		llmUntil time.Time
		vecOK    bool
		vecUntil time.Time
	}
}

func NewEngine(cfg EngineConfig) *Engine {
	return &Engine{
		cfg:        cfg,
		httpClient: &http.Client{Timeout: httpClientTimeout},
	}
}

// ---------------------------------------------------------------------------
// Availability probes (cached) — decide whether AI answers can be used
// ---------------------------------------------------------------------------

// cachedProbe remembers an availability check for probeCacheTTL so a chat turn
// never pays for two round-trips to Ollama/Qdrant before doing real work.
func (e *Engine) cachedProbe(kind string, check func() bool) bool {
	e.probe.mu.Lock()
	defer e.probe.mu.Unlock()
	now := time.Now()
	if kind == "llm" && now.Before(e.probe.llmUntil) {
		return e.probe.llmOK
	}
	if kind == "vec" && now.Before(e.probe.vecUntil) {
		return e.probe.vecOK
	}
	ok := check()
	if kind == "llm" {
		e.probe.llmOK = ok
		e.probe.llmUntil = now.Add(probeCacheTTL)
	} else {
		e.probe.vecOK = ok
		e.probe.vecUntil = now.Add(probeCacheTTL)
	}
	return ok
}

var probeClient = &http.Client{Timeout: probeTimeout}

// quickGet reports whether url answers 200 within the probe timeout.
func quickGet(ctx context.Context, url string) bool {
	reqCtx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, url, nil)
	if err != nil {
		return false
	}
	resp, err := probeClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// LLMHealthy reports whether Ollama answers within the cache window.
func (e *Engine) LLMHealthy(ctx context.Context) bool {
	return e.cachedProbe("llm", func() bool { return quickGet(ctx, e.cfg.OllamaURL+"/api/tags") })
}

// VectorHealthy reports whether the Qdrant collection is reachable.
func (e *Engine) VectorHealthy(ctx context.Context) bool {
	return e.cachedProbe("vec", func() bool { return quickGet(ctx, e.collectionURL("")) })
}

// collectionURL builds a Qdrant URL for the configured collection; suffix is
// appended verbatim ("" for the collection itself).
func (e *Engine) collectionURL(suffix string) string {
	return fmt.Sprintf("%s/collections/%s%s", e.cfg.QdrantURL, e.cfg.Collection, suffix)
}

// ---------------------------------------------------------------------------
// Vector search with the query text (embed → search → threshold)
// ---------------------------------------------------------------------------

// VectorSearchText embeds the query and runs Qdrant search; returns only
// chunks at or above the configured relevance threshold.
func (e *Engine) VectorSearchText(ctx context.Context, query string, limit int) ([]ScoredChunk, error) {
	if !e.VectorHealthy(ctx) {
		return nil, errors.New("qdrant unavailable")
	}
	embCtx, cancel := context.WithTimeout(ctx, embedTimeout)
	defer cancel()
	emb, err := e.getEmbedding(embCtx, query)
	if err != nil {
		return nil, err
	}
	sctx, scancel := context.WithTimeout(ctx, searchTimeout)
	defer scancel()
	chunks, err := e.searchKnowledge(sctx, emb, limit, "")
	if err != nil {
		return nil, err
	}
	var out []ScoredChunk
	for _, c := range chunks {
		if c.Score >= e.cfg.MinRelevanceScore {
			out = append(out, c)
		}
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// Grounded LLM answer — strictly from provided fragments, with citations
// ---------------------------------------------------------------------------

// Errors reported by GroundedAnswerStream.
var (
	// ErrGroundedRefused — the model refused (НЕТ_ОТВЕТА / explicit refusal)
	// before any token was shown to the user.
	ErrGroundedRefused = errors.New("grounded answer: model refused")
	// ErrGroundedInvalid — generation finished but failed post-validation;
	// `flushed` tells the caller whether tokens were already displayed.
	ErrGroundedInvalid = errors.New("grounded answer: failed validation")
)

// reEarlyRefusal detects a model refusal inside the decision buffer.
var reEarlyRefusal = regexp.MustCompile(`(?i)НЕТ_ОТВЕТА|не могу (ответить|предоставить|помочь)|в предоставленных материалах (нет|не содерж)|нет достаточн(ых|о) (данных|информац)|данных во фрагментах (нет|недостаточ)|не относится к (документообороту|моей специализации|компетенц)|не регламентируется (во фрагментах|в предоставленных)|нет однозначного ответа`)

// GroundedAnswerStream generates a grounded answer with live token delivery.
// Tokens are buffered until a decision point — the first citation {{s:…}} or
// ~200 runes — so a refusal never reaches the screen; if a refusal is
// detected the generation is cancelled immediately. After the decision point
// deltas flow to onToken live. The caller validates the accumulated text;
// `flushed` reports whether tokens were already shown.
func (e *Engine) GroundedAnswerStream(ctx context.Context, query string, chunks []ScoredChunk, onToken func(string)) (string, bool, error) {
	if len(chunks) == 0 {
		return "", false, errors.New("no context chunks")
	}
	system, user := groundedPrompt(query, chunks)
	gctx, cancel := context.WithTimeout(ctx, generateTimeout)
	defer cancel()

	var acc strings.Builder
	flushed := false
	aborted := false
	_, callErr := e.callLLMWithSystem(gctx, system, user, nil, func(delta string) {
		if aborted {
			return
		}
		acc.WriteString(delta)
		if !flushed {
			if reEarlyRefusal.MatchString(acc.String()) {
				aborted = true
				cancel() // stop wasting generation — nothing was shown yet
				return
			}
			if strings.Contains(acc.String(), "{{s:") || utf8.RuneCountInString(acc.String()) >= flushRunes {
				flushed = true
				if onToken != nil {
					onToken(acc.String())
				}
			}
			return
		}
		if onToken != nil {
			onToken(delta)
		}
	})
	full := acc.String()
	if aborted {
		return "", false, ErrGroundedRefused
	}
	if callErr != nil {
		return full, flushed, callErr
	}
	return full, flushed, nil
}

// groundedPrompt builds the strict system + user prompts for grounded answers.
func groundedPrompt(query string, chunks []ScoredChunk) (system, user string) {
	var ctxText strings.Builder
	for i, c := range chunks {
		ctxText.WriteString(fmt.Sprintf("--- ФРАГМЕНТ [%d] (section_id: %s) ---\n", i+1, c.SectionID))
		ctxText.WriteString(fmt.Sprintf("Документ: %s\nПункт: %s\nТекст: %s\n\n", c.DocumentTitle, c.Clause, c.Text))
	}
	system = `Ты — SancireAI, интеллектуальный помощник по документообороту (Инструкция по ДОУ № ОД-184, Матрица визирования, Методические рекомендации).
СТРОГИЕ ПРАВИЛА:
1. Отвечай ТОЛЬКО по переданным фрагментам. Запрещено добавлять факты из памяти модели.
2. Каждое нормативное утверждение или цитату оборачивай в тег {{s:section_id}}текст{{/s}}, где section_id берёшь ТОЛЬКО из списка фрагментов (без выдумывания). Первую цитату ставь в самом начале ответа.
3. Отвечай на русском, структурированно (Markdown: краткие списки, выделения). Сохраняй номера пунктов и даты из фрагментов дословно.
4. Если во фрагментах нет достаточного ответа на вопрос — ответь ровно одной строкой: НЕТ_ОТВЕТА
5. Не пересказывай весь фрагмент — отвечай на заданный вопрос.`
	user = fmt.Sprintf("ФРАГМЕНТЫ:\n%s\nВОПРОС ПОЛЬЗОВАТЕЛЯ:\n%s", ctxText.String(), query)
	return system, user
}

// ---------------------------------------------------------------------------
// Qdrant maintenance for admin-uploaded regulations
// ---------------------------------------------------------------------------

var regulationNS = uuid.MustParse("f6b1c1a0-0000-4000-8000-000000000001")

// pointID is the deterministic Qdrant point id for a section id, so re-pushing
// a regulation updates its points instead of duplicating them.
func pointID(sectionID string) string {
	return uuid.NewSHA1(regulationNS, []byte(sectionID)).String()
}

// PushChunks upserts chunks into Qdrant (embedding each one). Best-effort:
// errors are returned to the caller, which logs them without failing the
// admin operation — the local KB index remains the source of truth.
func (e *Engine) PushChunks(ctx context.Context, chunks []ScoredChunk) error {
	if !e.VectorHealthy(ctx) {
		return errors.New("qdrant unavailable")
	}
	type point struct {
		ID      string         `json:"id"`
		Vector  []float32      `json:"vector"`
		Payload map[string]any `json:"payload"`
	}
	var points []point
	for i, c := range chunks {
		ectx, cancel := context.WithTimeout(ctx, pushChunkTimeout)
		emb, err := e.getEmbedding(ectx, c.Text)
		cancel()
		if err != nil {
			return fmt.Errorf("embedding chunk %d: %w", i, err)
		}
		if len(emb) == 0 {
			continue
		}
		points = append(points, point{
			ID:     pointID(c.SectionID),
			Vector: emb,
			Payload: map[string]any{
				"chunk_id":       c.SectionID,
				"section_id":     c.SectionID,
				"document_title": c.DocumentTitle,
				"clause":         c.Clause,
				"title":          c.Title,
				"text":           c.Text,
				"updated_at":     c.UpdatedAt,
				"category":       c.Category,
				"version_id":     c.VersionID,
				"source":         "regulation",
			},
		})
	}
	if len(points) == 0 {
		return nil
	}
	// Create the collection when missing (dimension from the first embedding).
	if !quickGet(ctx, e.collectionURL("")) {
		if err := e.ensureCollection(ctx, len(points[0].Vector)); err != nil {
			return err
		}
	}
	for start := 0; start < len(points); start += pointBatch {
		end := min(start+pointBatch, len(points))
		body, err := json.Marshal(map[string]any{"points": points[start:end]})
		if err != nil {
			return err
		}
		if err := e.putJSON(ctx, e.collectionURL("/points?wait=true"), body, "upsert"); err != nil {
			return err
		}
	}
	return nil
}

// ensureCollection creates the Qdrant collection with a Cosine metric.
func (e *Engine) ensureCollection(ctx context.Context, dim int) error {
	body, err := json.Marshal(map[string]any{
		"vectors": map[string]any{"size": dim, "distance": vectorDistance},
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, e.collectionURL(""), bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := e.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("qdrant create collection: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		detail, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("qdrant create collection %d: %s", resp.StatusCode, string(detail))
	}
	return nil
}

// putJSON sends a JSON body to Qdrant and turns a non-2xx status into an error
// that carries the response body (read before the body is closed).
func (e *Engine) putJSON(ctx context.Context, url string, body []byte, op string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := e.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		detail, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("qdrant %s %d: %s", op, resp.StatusCode, string(detail))
	}
	return nil
}

// RemoveChunkPoints deletes regulation points by their section ids.
func (e *Engine) RemoveChunkPoints(ctx context.Context, sectionIDs []string) error {
	if len(sectionIDs) == 0 || !e.VectorHealthy(ctx) {
		return nil
	}
	ids := make([]string, 0, len(sectionIDs))
	for _, id := range sectionIDs {
		ids = append(ids, pointID(id))
	}
	body, err := json.Marshal(map[string]any{"ids": ids})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.collectionURL("/points/delete?wait=true"), bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := e.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		detail, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("qdrant delete %d: %s", resp.StatusCode, string(detail))
	}
	return nil
}

// LoadCorpus pushes the local knowledge index into Qdrant. limit caps the
// number of pushed chunks (0 = everything); it returns how many were pushed.
func (e *Engine) LoadCorpus(ctx context.Context, ix *kb.Index, limit int) (int, error) {
	if ix == nil {
		return 0, errors.New("nil index")
	}
	ids := ix.AllIDs()
	if limit > 0 && len(ids) > limit {
		ids = ids[:limit]
	}
	total := 0
	for start := 0; start < len(ids); start += corpusBatch {
		end := min(start+corpusBatch, len(ids))
		batch := make([]ScoredChunk, 0, end-start)
		for _, id := range ids[start:end] {
			c, ok := ix.Get(id)
			if !ok {
				continue
			}
			batch = append(batch, ScoredChunk{
				ChunkID: c.ID, SectionID: c.ID, DocumentTitle: c.DocumentTitle,
				Clause: c.Clause, Title: c.Title, Text: c.Text, Page: c.Page,
				UpdatedAt: c.UpdatedAt, VersionID: c.VersionID, Category: c.Category,
			})
		}
		if err := e.PushChunks(ctx, batch); err != nil {
			return total, err
		}
		total += len(batch)
	}
	return total, nil
}

// getEmbedding requests a vector from the local embedding model.
func (e *Engine) getEmbedding(ctx context.Context, text string) ([]float32, error) {
	reqBody, err := json.Marshal(map[string]any{
		"model":  e.cfg.EmbedModel,
		"prompt": text,
	})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.cfg.OllamaURL+"/api/embeddings", bytes.NewReader(reqBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := e.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ollama embeddings error (%d): %s", resp.StatusCode, string(body))
	}
	var result struct {
		Embedding []float32 `json:"embedding"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result.Embedding, nil
}

// searchKnowledge retrieves the most relevant chunks from Qdrant. A non-empty
// versionID restricts the search to one regulation edition.
func (e *Engine) searchKnowledge(ctx context.Context, queryVector []float32, limit int, versionID string) ([]ScoredChunk, error) {
	searchPayload := map[string]any{
		"vector":       queryVector,
		"limit":        limit,
		"with_payload": true,
	}
	if versionID != "" {
		searchPayload["filter"] = map[string]any{
			"must": []map[string]any{
				{
					"key": "version_id",
					"match": map[string]any{
						"value": versionID,
					},
				},
			},
		}
	}
	b, err := json.Marshal(searchPayload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.collectionURL("/points/search"), bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := e.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("qdrant search error (%d): %s", resp.StatusCode, string(b))
	}
	var qRes struct {
		Result []struct {
			Score   float64        `json:"score"`
			Payload map[string]any `json:"payload"`
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&qRes); err != nil {
		return nil, err
	}
	chunks := make([]ScoredChunk, 0, len(qRes.Result))
	for _, p := range qRes.Result {
		sc := ScoredChunk{Score: p.Score}
		payloadString(p.Payload, "chunk_id", &sc.ChunkID)
		payloadString(p.Payload, "section_id", &sc.SectionID)
		payloadString(p.Payload, "document_title", &sc.DocumentTitle)
		payloadString(p.Payload, "clause", &sc.Clause)
		payloadString(p.Payload, "title", &sc.Title)
		payloadString(p.Payload, "text", &sc.Text)
		payloadString(p.Payload, "updated_at", &sc.UpdatedAt)
		payloadString(p.Payload, "version_id", &sc.VersionID)
		payloadString(p.Payload, "category", &sc.Category)
		chunks = append(chunks, sc)
	}
	return chunks, nil
}

// payloadString copies an optional string field out of a Qdrant payload.
func payloadString(payload map[string]any, key string, dst *string) {
	if v, ok := payload[key].(string); ok {
		*dst = v
	}
}

// streamResponse emits small UTF-8-safe chunks for deterministic answers and
// graceful fallbacks too, rather than dumping the whole reply in one SSE event.
func streamResponse(tokenCallback func(string), text string) {
	if tokenCallback == nil || text == "" {
		return
	}
	runes := []rune(text)
	for start := 0; start < len(runes); start += streamRunes {
		end := min(start+streamRunes, len(runes))
		tokenCallback(string(runes[start:end]))
		if end < len(runes) {
			time.Sleep(streamDelay)
		}
	}
}

// truncateRunes cuts s to at most n runes, never splitting a UTF-8 character.
func truncateRunes(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n])
}

// Action names of the two chat actions served by the LLM pipeline. Every other
// turn is answered by the deterministic dialog engine.
const (
	ActionExplain = "explain"
	ActionExpert  = "expert"
)

const explainPromptFormat = `Объясни простыми, понятными словами следующий пункт регламента:

«%s»

Сделай ответ кратким, доброжелательным и структурированным.`

const expertReplyText = "Ваш вопрос зарегистрирован и передан специалисту Центра документационного обеспечения (ЦДОБР).\n\n" +
	"Эксперт рассмотрит нестандартную ситуацию и свяжется с вами в течение рабочего дня."

// GenerateAnswer serves the «Объяснить» and «Эксперт» chat actions: the first
// re-phrases the selected fragment with the local LLM, the second registers an
// expert escalation. Tokens are streamed through tokenCallback when present.
func (e *Engine) GenerateAnswer(ctx context.Context, query, action, selection string, tokenCallback func(string)) (string, MessageMeta, error) {
	switch action {
	case ActionExplain:
		return e.explainSelection(ctx, query, selection, tokenCallback)
	case ActionExpert:
		streamResponse(tokenCallback, expertReplyText)
		return expertReplyText, MessageMeta{
			Expert:    true,
			FollowUps: []string{"Порядок согласования приказа", "Открыть библиотеку шаблонов"},
		}, nil
	default:
		return "", MessageMeta{}, fmt.Errorf("unsupported chat action %q", action)
	}
}

// explainSelection re-tells a normative fragment in plain language. The LLM is
// best-effort: when Ollama fails, a template answer keeps the UI useful.
func (e *Engine) explainSelection(ctx context.Context, query, selection string, tokenCallback func(string)) (string, MessageMeta, error) {
	fragment := strings.TrimSpace(selection)
	if fragment == "" {
		fragment = strings.TrimSpace(query)
	}
	if fragment == "" {
		return "", MessageMeta{}, errors.New("explain: нет фрагмента для объяснения")
	}
	ectx, cancel := context.WithTimeout(ctx, explainTimeout)
	defer cancel()

	prompt := fmt.Sprintf(explainPromptFormat, fragment)
	ans, err := e.callLLM(ectx, prompt, nil, tokenCallback)
	if err == nil {
		return ans, MessageMeta{}, nil
	}
	if strings.TrimSpace(ans) != "" {
		// Partial text was already streamed — never append a second answer.
		log.Warn().Err(err).Msg("ollama explain interrupted, keeping the streamed part")
		return ans, MessageMeta{}, nil
	}
	log.Warn().Err(err).Msg("ollama explain failed, using the template fallback")
	snippet := truncateRunes(fragment, explainSnippetRunes)
	fallback := fmt.Sprintf("### Простое объяснение\n\nДанный пункт регламентирует: **%s**.\n\n"+
		"Главное требование: строгое соблюдение установленной последовательности и недопустимость пропуска обязательных этапов.", snippet)
	streamResponse(tokenCallback, fallback)
	return fallback, MessageMeta{}, nil
}

func (e *Engine) callLLM(ctx context.Context, prompt string, history []ChatMessage, tokenCallback func(string)) (string, error) {
	return e.callLLMWithSystem(ctx, "", prompt, history, tokenCallback)
}
func (e *Engine) callLLMWithSystem(ctx context.Context, systemPrompt, userMessage string, history []ChatMessage, tokenCallback func(string)) (string, error) {
	var msgs []map[string]string
	if systemPrompt != "" {
		msgs = append(msgs, map[string]string{"role": "system", "content": systemPrompt})
	}
	// The last history row is the just-persisted current user message; the prompt
	// below already contains it. Include only a bounded window of prior turns.
	end := len(history)
	if end > 0 && history[end-1].Role == "user" {
		end--
	}
	start := max(end-maxHistoryMsgs, 0)
	for _, message := range history[start:end] {
		if message.Role != "user" && message.Role != "assistant" {
			continue
		}
		msgs = append(msgs, map[string]string{
			"role":    message.Role,
			"content": truncateRunes(message.Content, maxHistoryRunes),
		})
	}
	msgs = append(msgs, map[string]string{"role": "user", "content": userMessage})
	reqBody := map[string]any{
		"model":    e.cfg.LLMModel,
		"messages": msgs,
		"stream":   tokenCallback != nil,
		"think":    false,
		"options": map[string]any{
			"temperature": 0.2,
			"num_predict": 1024,
		},
	}
	b, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.cfg.OllamaURL+"/api/chat", bytes.NewReader(b))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := e.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("ollama chat error (%d): %s", resp.StatusCode, string(b))
	}
	if tokenCallback != nil {
		// Stream tokens line by line
		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 64*1024), 2*1024*1024)
		var full strings.Builder
		streamDone := false
		for scanner.Scan() {
			if ctx.Err() != nil {
				return full.String(), ctx.Err()
			}
			line := scanner.Text()
			if strings.TrimSpace(line) == "" {
				continue
			}
			var chunk struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
				Done  bool   `json:"done"`
				Error string `json:"error"`
			}
			if err := json.Unmarshal([]byte(line), &chunk); err == nil {
				if chunk.Error != "" {
					return full.String(), fmt.Errorf("ollama stream error: %s", chunk.Error)
				}
				if chunk.Message.Content != "" {
					full.WriteString(chunk.Message.Content)
					tokenCallback(chunk.Message.Content)
				}
				streamDone = streamDone || chunk.Done
			}
		}
		if err := scanner.Err(); err != nil {
			return full.String(), fmt.Errorf("read ollama stream: %w", err)
		}
		if !streamDone {
			return full.String(), errors.New("ollama stream closed before done")
		}
		return full.String(), nil
	}
	var chatRes struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&chatRes); err != nil {
		return "", err
	}
	return chatRes.Message.Content, nil
}

// ---------------------------------------------------------------------------
// Live infrastructure statistics for the admin dashboard
// ---------------------------------------------------------------------------

// CollectionStats describes the Qdrant collection behind the vector index.
type CollectionStats struct {
	Available  bool   `json:"available"`
	Collection string `json:"collection"`
	Points     int64  `json:"points"`
	Dimension  int    `json:"dimension"`
	Distance   string `json:"distance"`
}

// LLMStats describes the local Ollama instance.
type LLMStats struct {
	Available  bool     `json:"available"`
	Model      string   `json:"model"`
	EmbedModel string   `json:"embedModel"`
	Models     []string `json:"models,omitempty"`
}

// CollectionStats reads the live collection info from Qdrant. An unreachable
// service is reported as Available=false rather than as an error: the dashboard
// must render even when the vector store is down.
func (e *Engine) CollectionStats(ctx context.Context) CollectionStats {
	stats := CollectionStats{Collection: e.cfg.Collection, Distance: vectorDistance}
	if !e.VectorHealthy(ctx) {
		return stats
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, e.collectionURL(""), nil)
	if err != nil {
		return stats
	}
	resp, err := e.httpClient.Do(req)
	if err != nil {
		return stats
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return stats
	}
	var info struct {
		Result struct {
			PointsCount int64 `json:"points_count"`
			Config      struct {
				Params struct {
					Vectors json.RawMessage `json:"vectors"`
				} `json:"params"`
			} `json:"config"`
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return stats
	}
	stats.Available = true
	stats.Points = info.Result.PointsCount
	stats.Dimension, stats.Distance = vectorParams(info.Result.Config.Params.Vectors)
	return stats
}

// vectorParams reads size/distance either from a single unnamed vector config
// or from the first named vector.
func vectorParams(raw json.RawMessage) (int, string) {
	if len(raw) == 0 {
		return 0, vectorDistance
	}
	var single struct {
		Size     int    `json:"size"`
		Distance string `json:"distance"`
	}
	if err := json.Unmarshal(raw, &single); err == nil && single.Size > 0 {
		return single.Size, single.Distance
	}
	var named map[string]struct {
		Size     int    `json:"size"`
		Distance string `json:"distance"`
	}
	if err := json.Unmarshal(raw, &named); err == nil {
		for _, v := range named {
			if v.Size > 0 {
				return v.Size, v.Distance
			}
		}
	}
	return 0, vectorDistance
}

// LLMStats lists the models available on the local Ollama instance.
func (e *Engine) LLMStats(ctx context.Context) LLMStats {
	stats := LLMStats{Model: e.cfg.LLMModel, EmbedModel: e.cfg.EmbedModel}
	if !e.LLMHealthy(ctx) {
		return stats
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, e.cfg.OllamaURL+"/api/tags", nil)
	if err != nil {
		return stats
	}
	resp, err := e.httpClient.Do(req)
	if err != nil {
		return stats
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return stats
	}
	var tags struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tags); err != nil {
		return stats
	}
	stats.Available = true
	for _, m := range tags.Models {
		stats.Models = append(stats.Models, m.Name)
	}
	return stats
}
