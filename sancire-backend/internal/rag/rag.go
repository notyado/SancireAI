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
	"unicode/utf8"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"sancireai/backend/internal/kb"
)

type SourceRef struct {
	ID            string `json:"id"`
	DocumentTitle string `json:"documentTitle"`
	Clause        string `json:"clause"`
	Excerpt       string `json:"excerpt"`
	UpdatedAt     string `json:"updatedAt"`
	ParagraphID   string `json:"paragraphId,omitempty"`
}
type ProcessStep struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Deadline    string `json:"deadline,omitempty"`
	Department  string `json:"department,omitempty"`
}
type ProcessDiagram struct {
	Title string        `json:"title"`
	Steps []ProcessStep `json:"steps"`
}
type GeneratedDocument struct {
	Title   string `json:"title"`
	Kind    string `json:"kind"`
	Preview string `json:"preview"`
	Body    string `json:"body"`
}
type AttachedFile struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
	Type string `json:"type"`
}
type MessageMeta struct {
	Sources    []SourceRef        `json:"sources,omitempty"`
	Process    *ProcessDiagram    `json:"process,omitempty"`
	Document   *GeneratedDocument `json:"document,omitempty"`
	NotFound   bool               `json:"notFound,omitempty"`
	FollowUps  []string           `json:"followUps,omitempty"`
	Clarifying bool               `json:"clarifying,omitempty"`
	Files      []AttachedFile     `json:"files,omitempty"`
	Expert     bool               `json:"expert,omitempty"`
}
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
	Category      string  `json:"category"`
}
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}
type EngineConfig struct {
	QdrantURL         string
	Collection        string
	OllamaURL         string
	LLMModel          string
	EmbedModel        string
	MinRelevanceScore float64
}
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
		cfg: cfg,
		httpClient: &http.Client{
			Timeout: 240 * time.Second,
		},
	}
}

// ---------------------------------------------------------------------------
// Availability probes (cached) — decide whether AI answers can be used
// ---------------------------------------------------------------------------

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
		e.probe.llmUntil = now.Add(30 * time.Second)
	} else {
		e.probe.vecOK = ok
		e.probe.vecUntil = now.Add(30 * time.Second)
	}
	return ok
}

func quickGet(url string) bool {
	client := &http.Client{Timeout: 1500 * time.Millisecond}
	ctx, cancel := context.WithTimeout(context.Background(), 1600*time.Millisecond)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false
	}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// LLMHealthy reports whether Ollama answers within the cache window.
func (e *Engine) LLMHealthy(ctx context.Context) bool {
	_ = ctx
	return e.cachedProbe("llm", func() bool { return quickGet(e.cfg.OllamaURL + "/api/tags") })
}

// VectorHealthy reports whether Qdrant is reachable.
func (e *Engine) VectorHealthy(ctx context.Context) bool {
	_ = ctx
	return e.cachedProbe("vec", func() bool {
		return quickGet(fmt.Sprintf("%s/collections/%s", e.cfg.QdrantURL, e.cfg.Collection))
	})
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
	embCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	emb, err := e.GetEmbedding(embCtx, query)
	if err != nil {
		return nil, err
	}
	sctx, scancel := context.WithTimeout(ctx, 8*time.Second)
	defer scancel()
	chunks, err := e.SearchKnowledge(sctx, emb, limit, "")
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

// GroundedAnswer asks the local LLM to answer using ONLY the given chunks.
// The call is buffered (no token streaming) so the caller can validate the
// result — citations must reference provided ids, otherwise the answer is
// rejected and the deterministic composer takes over.
func (e *Engine) GroundedAnswer(ctx context.Context, query string, chunks []ScoredChunk) (string, error) {
	if len(chunks) == 0 {
		return "", errors.New("no context chunks")
	}
	system, user := groundedPrompt(query, chunks)
	gctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	return e.callLLMWithSystem(gctx, system, user, nil, nil)
}

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
	gctx, cancel := context.WithTimeout(ctx, 90*time.Second)
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
			if strings.Contains(acc.String(), "{{s:") || utf8.RuneCountInString(acc.String()) >= 200 {
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

// PointID is the deterministic Qdrant point id for a section id.
func PointID(sectionID string) string {
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
		ectx, cancel := context.WithTimeout(ctx, 10*time.Second)
		emb, err := e.GetEmbedding(ectx, c.Text)
		cancel()
		if err != nil {
			return fmt.Errorf("embedding chunk %d: %w", i, err)
		}
		if len(emb) == 0 {
			continue
		}
		points = append(points, point{
			ID:     PointID(c.SectionID),
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
				"version_id":     c.UpdatedAt,
				"source":         "regulation",
			},
		})
	}
	if len(points) == 0 {
		return nil
	}
	// Create the collection when missing (dim from the first embedding).
	var first []float32 = points[0].Vector
	if !quickGet(fmt.Sprintf("%s/collections/%s", e.cfg.QdrantURL, e.cfg.Collection)) {
		createBody, _ := json.Marshal(map[string]any{
			"vectors": map[string]any{"size": len(first), "distance": "Cosine"},
		})
		req, _ := http.NewRequestWithContext(ctx, http.MethodPut,
			e.cfg.QdrantURL+"/collections/"+e.cfg.Collection, bytes.NewReader(createBody))
		req.Header.Set("Content-Type", "application/json")
		if resp, err := e.httpClient.Do(req); err == nil {
			resp.Body.Close()
		}
	}
	const batch = 64
	for start := 0; start < len(points); start += batch {
		end := start + batch
		if end > len(points) {
			end = len(points)
		}
		body, _ := json.Marshal(map[string]any{"points": points[start:end]})
		req, _ := http.NewRequestWithContext(ctx, http.MethodPut,
			fmt.Sprintf("%s/collections/%s/points?wait=true", e.cfg.QdrantURL, e.cfg.Collection), bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := e.httpClient.Do(req)
		if err != nil {
			return err
		}
		resp.Body.Close()
		if resp.StatusCode >= 300 {
			b, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("qdrant upsert %d: %s", resp.StatusCode, string(b))
		}
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
		ids = append(ids, PointID(id))
	}
	body, _ := json.Marshal(map[string]any{"ids": ids})
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf("%s/collections/%s/points/delete?wait=true", e.cfg.QdrantURL, e.cfg.Collection), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := e.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("qdrant delete %d: %s", resp.StatusCode, string(b))
	}
	return nil
}

// LoadCorpus pushes the local knowledge index into Qdrant (used by the
// qdrantload command and on regulation activation).
func (e *Engine) LoadCorpus(ix *kb.Index, limit int) (int, error) {
	if ix == nil {
		return 0, errors.New("nil index")
	}
	ids := ix.AllIDs()
	total := 0
	for start := 0; start < len(ids); start += 500 {
		end := start + 500
		if end > len(ids) {
			end = len(ids)
		}
		var batch []ScoredChunk
		for _, id := range ids[start:end] {
			c, ok := ix.Get(id)
			if !ok {
				continue
			}
			batch = append(batch, ScoredChunk{
				ChunkID: c.ID, SectionID: c.ID, DocumentTitle: c.DocumentTitle,
				Clause: c.Clause, Title: c.Title, Text: c.Text, Page: c.Page,
				UpdatedAt: c.UpdatedAt, Category: c.Category,
			})
		}
		if err := e.PushChunks(ctxBackground(), batch); err != nil {
			return total, err
		}
		total += len(batch)
		if limit > 0 && total >= limit {
			break
		}
	}
	return total, nil
}

func ctxBackground() context.Context { return context.Background() }

// GetEmbedding requests a vector from Ollama
func (e *Engine) GetEmbedding(ctx context.Context, text string) ([]float32, error) {
	reqBody, _ := json.Marshal(map[string]any{
		"model":  e.cfg.EmbedModel,
		"prompt": text,
	})
	req, err := http.NewRequestWithContext(ctx, "POST", e.cfg.OllamaURL+"/api/embeddings", bytes.NewReader(reqBody))
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

// SearchKnowledge retrieves the most relevant chunks from Qdrant
func (e *Engine) SearchKnowledge(ctx context.Context, queryVector []float32, limit int, versionID string) ([]ScoredChunk, error) {
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
	b, _ := json.Marshal(searchPayload)
	req, err := http.NewRequestWithContext(ctx, "POST", fmt.Sprintf("%s/collections/%s/points/search", e.cfg.QdrantURL, e.cfg.Collection), bytes.NewReader(b))
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
	var chunks []ScoredChunk
	for _, p := range qRes.Result {
		sc := ScoredChunk{
			Score: p.Score,
		}
		if v, ok := p.Payload["chunk_id"].(string); ok {
			sc.ChunkID = v
		}
		if v, ok := p.Payload["section_id"].(string); ok {
			sc.SectionID = v
		}
		if v, ok := p.Payload["document_title"].(string); ok {
			sc.DocumentTitle = v
		}
		if v, ok := p.Payload["clause"].(string); ok {
			sc.Clause = v
		}
		if v, ok := p.Payload["title"].(string); ok {
			sc.Title = v
		}
		if v, ok := p.Payload["text"].(string); ok {
			sc.Text = v
		}
		if v, ok := p.Payload["updated_at"].(string); ok {
			sc.UpdatedAt = v
		}
		if v, ok := p.Payload["category"].(string); ok {
			sc.Category = v
		}
		chunks = append(chunks, sc)
	}
	return chunks, nil
}

// streamResponse emits small UTF-8-safe chunks for deterministic answers and
// graceful fallbacks too, rather than dumping the whole reply in one SSE event.
func streamResponse(tokenCallback func(string), text string) {
	if tokenCallback == nil || text == "" {
		return
	}
	runes := []rune(text)
	const chunkSize = 24
	for start := 0; start < len(runes); start += chunkSize {
		end := start + chunkSize
		if end > len(runes) {
			end = len(runes)
		}
		tokenCallback(string(runes[start:end]))
		if end < len(runes) {
			time.Sleep(8 * time.Millisecond)
		}
	}
}

// GenerateAnswer handles the full RAG pipeline with streaming SSE callback
func (e *Engine) GenerateAnswer(
	ctx context.Context,
	userQuery string,
	history []ChatMessage,
	action string,
	selection string,
	activeVersion string,
	tokenCallback func(token string),
) (string, MessageMeta, error) {
	norm := strings.ToLower(strings.TrimSpace(userQuery))
	// 1. Handle conversational greetings & etiquette directly for instant response
	if isGreeting(norm) {
		greeting := "Здравствуйте! Я — **SancireAI**, ваш интеллектуальный помощник по внутренним регламентам и документационному обеспечению управления Банка России.\n\nЯ могу помочь вам:\n- 🔍 **Найти ответы на вопросы** по согласованию, срокам и требованиям Инструкции по ДОУ № ОД-184;\n- 📑 **Сформировать типовой документ** по одному из 7 утвержденных модулей;\n- 🛡️ **Проверить готовый проект** приказа или распоряжения и рассчитать полный маршрут визирования по Матрице.\n\nКакой вопрос вас интересует?"
		if tokenCallback != nil {
			streamResponse(tokenCallback, greeting)
		}
		meta := MessageMeta{
			FollowUps: []string{
				"Как согласовать внутренний документ?",
				"Оформить служебную записку",
				"Сроки командировки по России",
			},
		}
		return greeting, meta, nil
	}
	if isSmallTalk(norm) {
		reply := "Увы, обсуждение отвлеченных тем не входит в мой функционал. Я специализируюсь исключительно на внутренних регламентах, согласовании и подготовке документов Банка России.\n\nЗадайте вопрос по делопроизводству, срокам или выберите нужный шаблон."
		if tokenCallback != nil {
			streamResponse(tokenCallback, reply)
		}
		meta := MessageMeta{
			FollowUps: []string{
				"Порядок согласования приказа",
				"Оформить заявление на отпуск",
			},
		}
		return reply, meta, nil
	}
	// 2. Action: explain selection
	if action == "explain" && selection != "" {
		prompt := fmt.Sprintf("Объясни простыми, понятными словами следующий пункт регламента:\n\n«%s»\n\nСделай ответ кратким, доброжелательным и структурированным.", selection)
		ans, err := e.callLLM(ctx, prompt, nil, tokenCallback)
		if err != nil && strings.TrimSpace(ans) != "" {
			return ans, MessageMeta{}, nil
		}
		if err != nil {
			fallback := fmt.Sprintf("### Простое объяснение\n\nДанный пункт регламентирует: **%s**.\n\nГлавное требование: строгое соблюдение установленной последовательности и недопустимость пропуска обязательных этапов.", selection[:min(len(selection), 200)])
			if tokenCallback != nil {
				streamResponse(tokenCallback, fallback)
			}
			return fallback, MessageMeta{}, nil
		}
		return ans, MessageMeta{}, nil
	}
	// 3. Action: expert escalation
	if action == "expert" {
		reply := "Ваш вопрос зарегистрирован и передан специалисту Центра документационного обеспечения (ЦДОБР).\n\nЭксперт рассмотрит нестандартную ситуацию и свяжется с вами в течение рабочего дня."
		if tokenCallback != nil {
			streamResponse(tokenCallback, reply)
		}
		meta := MessageMeta{
			Expert: true,
			FollowUps: []string{
				"Порядок согласования приказа",
				"Открыть библиотеку шаблонов",
			},
		}
		return reply, meta, nil
	}
	// 4. Retrieve knowledge chunks via Qdrant
	emb, err := e.GetEmbedding(ctx, userQuery)
	if err != nil {
		log.Warn().Err(err).Msg("Ollama embedding failed, attempting graceful fallback")
		return e.fallbackReply(userQuery, nil, tokenCallback)
	}
	chunks, err := e.SearchKnowledge(ctx, emb, 3, activeVersion)
	if err != nil {
		log.Warn().Err(err).Msg("Qdrant search failed, attempting fallback")
		return e.fallbackReply(userQuery, nil, tokenCallback)
	}
	// 5. Evaluate relevance score for Safe Refusal
	maxScore := 0.0
	for _, c := range chunks {
		if c.Score > maxScore {
			maxScore = c.Score
		}
	}
	// Safe Refusal if no confidence (score < 0.65 or empty)
	if len(chunks) == 0 || maxScore < e.cfg.MinRelevanceScore {
		log.Info().Float64("maxScore", maxScore).Msg("Score below threshold, executing natural Safe Refusal")
		return e.safeRefusalLLM(ctx, userQuery, chunks, tokenCallback)
	}
	// 6. Build prompt with context
	prompt := e.buildPrompt(userQuery, chunks, history)
	// 7. Call LLM with streaming tokens
	ans, err := e.callLLM(ctx, prompt, history, tokenCallback)
	if err != nil {
		log.Warn().Err(err).Msg("Ollama LLM call failed")
		// Do not append a complete fallback after already-streamed partial output.
		if strings.TrimSpace(ans) != "" {
			return ans, e.buildMetaFromAnswer(ans, chunks), nil
		}
		return e.fallbackReply(userQuery, chunks, tokenCallback)
	}
	// 8. Construct MessageMeta with synchronized sources
	meta := e.buildMetaFromAnswer(ans, chunks)
	return ans, meta, nil
}
func (e *Engine) safeRefusalLLM(ctx context.Context, query string, chunks []ScoredChunk, tokenCallback func(string)) (string, MessageMeta, error) {
	systemPrompt := `Ты — SancireAI, дружелюбный корпоративный консультант Банка России по регламентам.
Пользователь задал вопрос, на который в предоставленных регламентах и Инструкции по ДОУ НЕТ достоверного ответа.
ТВОЯ ЗАДАЧА:
Ответь живо, вежливо и естественно от первого лица. Поясни, что в доступной нормативной базе Банка России этот вопрос не регламентирован.
НЕ используй заученные роботизированные фразы. Сформулируй отказ своими словами, подчеркнув, что для исключения дезинформации надежнее обратиться напрямую к уполномоченному эксперту ДОУ.
Оформи ответ в Markdown.`
	refusalCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	resp, err := e.callLLMWithSystem(refusalCtx, systemPrompt, query, nil, tokenCallback)
	if err != nil && strings.TrimSpace(resp) != "" {
		return resp, MessageMeta{NotFound: true}, nil
	}
	if err != nil {
		naturalRefusal := "К сожалению, в действующих нормативных актах и Инструкции по ДОУ Банка России данный вопрос не регламентирован. Чтобы избежать неточных толкований, рекомендую передать обращение на рассмотрение эксперту ДОУ."
		if tokenCallback != nil {
			streamResponse(tokenCallback, naturalRefusal)
		}
		return naturalRefusal, MessageMeta{NotFound: true, FollowUps: []string{"Задать вопрос эксперту", "Порядок согласования"}}, nil
	}
	meta := MessageMeta{
		NotFound: true,
		FollowUps: []string{
			"Задать вопрос эксперту",
			"Порядок согласования приказа",
			"Открыть библиотеку шаблонов",
		},
	}
	return resp, meta, nil
}
func (e *Engine) fallbackReply(query string, chunks []ScoredChunk, tokenCallback func(string)) (string, MessageMeta, error) {
	var sb strings.Builder
	sb.WriteString("Модуль расширенного анализа временно испытывает высокую нагрузку, но я нашла точные выдержки из регламентов по вашему вопросу:\n\n")
	var sources []SourceRef
	for i, c := range chunks {
		if i >= 3 {
			break
		}
		tag := fmt.Sprintf("{{s:%s}}%s: %s{{/s}}", c.SectionID, c.Clause, c.Text)
		sb.WriteString(fmt.Sprintf("- %s\n\n", tag))
		sources = append(sources, SourceRef{
			ID:            c.SectionID,
			DocumentTitle: c.DocumentTitle,
			Clause:        c.Clause,
			Excerpt:       c.Text,
			UpdatedAt:     c.UpdatedAt,
		})
	}
	if len(sources) == 0 {
		sb.WriteString("Рекомендуется повторить запрос через несколько секунд или обратиться к специалисту ДОУ.")
	}
	ans := sb.String()
	if tokenCallback != nil {
		streamResponse(tokenCallback, ans)
	}
	return ans, MessageMeta{
		Sources: sources,
		FollowUps: []string{
			"Задать вопрос эксперту",
			"Порядок согласования",
		},
	}, nil
}
func (e *Engine) buildPrompt(userQuery string, chunks []ScoredChunk, history []ChatMessage) string {
	var ctxText strings.Builder
	for i, c := range chunks {
		ctxText.WriteString(fmt.Sprintf("--- ФРАГМЕНТ [%d] (ID секции: %s) ---\n", i+1, c.SectionID))
		ctxText.WriteString(fmt.Sprintf("Документ: %s\n", c.DocumentTitle))
		ctxText.WriteString(fmt.Sprintf("Пункт: %s\n", c.Clause))
		ctxText.WriteString(fmt.Sprintf("Текст: %s\n\n", c.Text))
	}
	prompt := fmt.Sprintf(`Ты — SancireAI, официальный интеллектуальный помощник Банка России по регламентам и ДОУ.
Отвечай вежливо, четко, на чистом грамотном русском языке. Обязательно используй структурированный Markdown (заголовки ##, списки -, жирный шрифт).
ПРАВИЛО ЦИТИРОВАНИЯ (КРИТИЧЕСКИ ВАЖНО):
Каждый нормативный факт, утверждение или цитату оборачивай в специальный тег {{s:<sectionId>}}текст утверждения или цитаты{{/s}}, где <sectionId> — точный ID секции из фрагментов контекста ниже!
Например: «{{s:docflow-4-2}}Лист согласования формируется исполнителем до направления по маршруту{{/s}}».
Не выдумывай sectionId — бери только из переданных фрагментов.
КОНТЕКСТ РЕГЛАМЕНТОВ:
%s
ВОПРОС ПОЛЬЗОВАТЕЛЯ:
%s
Ответь развернуто, структурированно, со ссылками на пункты и правила. Не выводи блоки внутренних размышлений или теги рассуждений, сразу пиши итоговый ответ.`, ctxText.String(), userQuery)
	return prompt
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
	start := end - 12
	if start < 0 {
		start = 0
	}
	for _, message := range history[start:end] {
		if message.Role != "user" && message.Role != "assistant" {
			continue
		}
		content := message.Content
		if len(content) > 4000 {
			content = content[:4000]
		}
		msgs = append(msgs, map[string]string{"role": message.Role, "content": content})
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
	b, _ := json.Marshal(reqBody)
	req, err := http.NewRequestWithContext(ctx, "POST", e.cfg.OllamaURL+"/api/chat", bytes.NewReader(b))
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
func (e *Engine) buildMetaFromAnswer(ans string, chunks []ScoredChunk) MessageMeta {
	meta := MessageMeta{}
	// Extract cited sectionIds using regex
	re := regexp.MustCompile(`\{\{s:([^}]+)\}\}`)
	matches := re.FindAllStringSubmatch(ans, -1)
	usedIDs := make(map[string]bool)
	for _, m := range matches {
		if len(m) > 1 {
			usedIDs[m[1]] = true
		}
	}
	var sources []SourceRef
	for _, c := range chunks {
		if usedIDs[c.SectionID] {
			sources = append(sources, SourceRef{
				ID:            c.SectionID,
				DocumentTitle: c.DocumentTitle,
				Clause:        c.Clause,
				Excerpt:       c.Text,
				UpdatedAt:     c.UpdatedAt,
				ParagraphID:   c.SectionID,
			})
			delete(usedIDs, c.SectionID)
		}
	}
	// If no tags were inserted by model but we had high confidence chunks, include top 1
	if len(sources) == 0 && len(chunks) > 0 && chunks[0].Score >= e.cfg.MinRelevanceScore {
		sources = append(sources, SourceRef{
			ID:            chunks[0].SectionID,
			DocumentTitle: chunks[0].DocumentTitle,
			Clause:        chunks[0].Clause,
			Excerpt:       chunks[0].Text,
			UpdatedAt:     chunks[0].UpdatedAt,
			ParagraphID:   chunks[0].SectionID,
		})
	}
	meta.Sources = sources
	// Add dynamic followups based on content
	low := strings.ToLower(ans)
	if strings.Contains(low, "согласован") || strings.Contains(low, "визир") {
		meta.Process = &ProcessDiagram{
			Title: "Маршрут согласования проекта документа",
			Steps: []ProcessStep{
				{ID: "step-1", Title: "Подготовка", Description: "Формирование проекта и листа согласования исполнителем", Deadline: "День 1"},
				{ID: "step-2", Title: "Экспертиза ДОУ", Description: "Лингвистический контроль корректором ДОУ", Deadline: "До 2 дней", Department: "Департамент ДОУ"},
				{ID: "step-3", Title: "Визирование", Description: "Согласование департаментами по Матрице", Deadline: "До 3–5 дней"},
				{ID: "step-4", Title: "Подписание", Description: "Утверждение руководством Банка России", Deadline: "По регламенту"},
				{ID: "step-5", Title: "Регистрация", Description: "Регистрация в СЭД и передача в дело", Deadline: "1 день", Department: "Департамент ДОУ"},
			},
		}
		meta.FollowUps = []string{
			"Сроки согласования на каждом этапе",
			"Оформить служебную записку",
			"Открыть Инструкция по документообороту",
		}
	} else if strings.Contains(low, "командир") {
		meta.FollowUps = []string{
			"Сроки предоставления авансового отчета",
			"Оформить служебное задание на командировку",
		}
	} else if strings.Contains(low, "отпуск") {
		meta.FollowUps = []string{
			"Оформить заявление на отпуск",
			"Сроки согласования отпуска",
		}
	} else {
		meta.FollowUps = []string{
			"Порядок согласования внутреннего документа",
			"Оформить служебную записку",
			"Открыть библиотеку шаблонов",
		}
	}
	return meta
}
func isGreeting(s string) bool {
	greetings := []string{"привет", "здравствуй", "добрый день", "доброе утро", "добрый вечер", "салют", "хай"}
	for _, g := range greetings {
		if strings.HasPrefix(s, g) || s == g {
			return true
		}
	}
	return false
}
func isSmallTalk(s string) bool {
	talk := []string{"как дела", "как жизнь", "кто ты", "расскажи анекдот", "погода", "чем занимаешься"}
	for _, t := range talk {
		if strings.Contains(s, t) {
			return true
		}
	}
	return false
}
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
