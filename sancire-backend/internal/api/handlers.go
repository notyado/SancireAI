// Package api exposes the HTTP surface of SancireAI: chat, conversations,
// document generation and audit, the regulation lifecycle and health checks.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"sancireai/backend/internal/config"
	"sancireai/backend/internal/dialog"
	"sancireai/backend/internal/docx"
	"sancireai/backend/internal/extract"
	"sancireai/backend/internal/kb"
	"sancireai/backend/internal/rag"
	"sancireai/backend/internal/store"
)

const (
	// serviceVersion is reported by GET /api/health.
	serviceVersion = "1.0.0"
	// defaultConversationTitle is used when a thread has no usable first message.
	defaultConversationTitle = "Новый диалог"
	// conversationTitleRunes caps the generated thread title.
	conversationTitleRunes = 52

	// Background Qdrant sync must never block an admin action, but it also must
	// not outlive the process by much.
	qdrantRemoveTimeout  = 60 * time.Second
	qdrantCleanupTimeout = 30 * time.Second
	qdrantPushTimeout    = 5 * time.Minute

	// adminStatsTimeout bounds the infrastructure probes of /api/admin/stats.
	adminStatsTimeout = 4 * time.Second
)

// Handler bundles everything the HTTP layer needs.
type Handler struct {
	cfg    config.Config
	store  *store.Store
	rag    *rag.Engine
	dialog *dialog.Engine
}

// NewHandler wires the store, the RAG engine and the dialog engine together.
func NewHandler(cfg config.Config, s *store.Store, r *rag.Engine) *Handler {
	// Dynamically load extra modules if custom definitions exist
	_ = docx.LoadModulesFromFile(filepath.Join(cfg.DataDir, "modules.json"))
	_ = docx.LoadModulesFromFile(filepath.Join(cfg.CorpusDir, "modules.json"))

	dlg := dialog.NewEngine(cfg.TemplatesDir, cfg.CorpusDir)
	dlg.UseLLM(r)
	h := &Handler{
		cfg:    cfg,
		store:  s,
		rag:    r,
		dialog: dlg,
	}
	h.refreshRegulationIndex()
	return h
}

// refreshRegulationIndex reloads ACTIVE regulation chunks into the KB index.
func (h *Handler) refreshRegulationIndex() {
	rows, err := h.store.ActiveRegulationChunks()
	if err != nil {
		log.Warn().Err(err).Msg("active regulations load failed")
		return
	}
	chunks := make([]*kb.Chunk, 0, len(rows))
	for _, c := range rows {
		chunks = append(chunks, &kb.Chunk{
			ID:            c.ID,
			DocumentTitle: c.DocumentTitle,
			Clause:        c.Clause,
			Title:         c.Title,
			Text:          c.Text,
			Page:          c.Page,
			UpdatedAt:     c.UpdatedAt,
			Category:      c.Kind,
		})
	}
	h.dialog.ReloadRegulations(chunks)
}

// regulationToScored converts stored chunks for Qdrant pushes.
func regulationToScored(rows []store.RegulationChunk) []rag.ScoredChunk {
	out := make([]rag.ScoredChunk, 0, len(rows))
	for _, c := range rows {
		out = append(out, rag.ScoredChunk{
			ChunkID:       c.ID,
			SectionID:     c.ID,
			DocumentTitle: c.DocumentTitle,
			Clause:        c.Clause,
			Title:         c.Title,
			Text:          c.Text,
			UpdatedAt:     c.UpdatedAt,
			Category:      c.Kind,
		})
	}
	return out
}

// ---------------------------------------------------------------------------
// Shared response helpers
// ---------------------------------------------------------------------------

// writeJSON answers with a JSON body and the given status.
func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

// writeError answers with {"error": code} plus an optional human-readable
// message. Unlike http.Error the body is real JSON, so the frontend can show
// the message instead of a parse error.
func writeError(w http.ResponseWriter, status int, code, message string) {
	body := map[string]string{"error": code}
	if message != "" {
		body["message"] = message
	}
	writeJSON(w, status, body)
}

// writeDocx streams a generated document as a download.
func writeDocx(w http.ResponseWriter, filename string, data []byte) {
	w.Header().Set("Content-Type", docxContentType)
	w.Header().Set("Content-Disposition", contentDisposition(filename))
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

const docxContentType = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"

// contentDisposition builds an RFC 5987 header so Cyrillic file names survive
// the download in every browser.
func contentDisposition(filename string) string {
	return "attachment; filename*=UTF-8''" + url.PathEscape(filename)
}

// conversationTitle derives a thread title from the first message, falling back
// to the first attachment name and finally to the neutral default.
func conversationTitle(text string, files []dialog.File) string {
	if title := truncateTitle(text); title != "" {
		return title
	}
	if len(files) > 0 {
		if title := truncateTitle(files[0].Name); title != "" {
			return title
		}
	}
	return defaultConversationTitle
}

// truncateTitle normalises whitespace and caps the length; it returns an empty
// string when there is nothing to title.
func truncateTitle(text string) string {
	s := strings.Join(strings.Fields(strings.ReplaceAll(strings.TrimSpace(text), "\n", " ")), " ")
	if s == "" {
		return ""
	}
	runes := []rune(s)
	if len(runes) > conversationTitleRunes {
		return string(runes[:conversationTitleRunes]) + "…"
	}
	return s
}

// truncateTitleOr is truncateTitle with a fallback for an empty result.
func truncateTitleOr(text, fallback string) string {
	if title := truncateTitle(text); title != "" {
		return title
	}
	return fallback
}

// sizeLimitMB renders a byte limit as the whole megabytes shown in error text.
func sizeLimitMB(limit int64) int64 { return limit / (1 << 20) }

// ---------------------------------------------------------------------------
// Auth
// ---------------------------------------------------------------------------

func (h *Handler) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "")
		return
	}
	user, err := h.store.ValidateUser(strings.TrimSpace(req.Username), req.Password)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid_credentials", err.Error())
		return
	}
	token, err := GenerateToken(user, h.cfg.JWTSecret, h.cfg.TokenTTL)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "token_generation_failed", "")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"token": token,
		"user": map[string]any{
			"id":       user.ID,
			"username": user.Username,
			"role":     user.Role,
			"name":     user.Name,
		},
	})
}
func (h *Handler) handleMe(w http.ResponseWriter, r *http.Request) {
	claims, ok := r.Context().Value(UserContextKey).(*Claims)
	if !ok || claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"user": map[string]any{
			"id":       claims.UserID,
			"username": claims.Username,
			"role":     claims.Role,
			"name":     claims.Name,
		},
	})
}
// chatBody is the POST /api/chat request payload.
type chatBody struct {
	ConversationID string        `json:"conversationId"`
	Content        string        `json:"content"`
	Files          []dialog.File `json:"files"`
	Action         string        `json:"action"`
	Selection      string        `json:"selection"`
}

const actionClarify = "clarify"

// generateTurn routes one chat turn either to the deterministic dialog engine
// (wizard / knowledge / audit) or to the local LLM for the «Объяснить» and
// «Эксперт» actions. It persists wizard draft state and generated artifacts.
func (h *Handler) generateTurn(r *http.Request, conv *store.Conversation, userContent string, body chatBody, history []rag.ChatMessage, token func(string)) (string, any, error) {
	if body.Action == rag.ActionExplain || body.Action == rag.ActionExpert {
		if h.rag == nil {
			return "", nil, errors.New("LLM engine is not configured")
		}
		return h.rag.GenerateAnswer(r.Context(), userContent, body.Action, body.Selection, token)
	}
	// "Уточнить" deep-dives the selected fragment: query the KB with the
	// selection itself so the answer stays grounded in that passage.
	dText := userContent
	if body.Action == actionClarify && body.Selection != "" {
		dText = body.Selection
	}
	out := h.dialog.Respond(dialog.Request{
		Ctx:     r.Context(),
		Text:    dText,
		Files:   body.Files,
		Draft:   conv.Draft,
		Token:   token,
		History: history,
	})
	if out.Artifact != nil {
		id := uuid.NewString()
		if err := h.store.SaveArtifact(id, out.Artifact.FileName, out.Artifact.Title, out.Artifact.Kind, out.Artifact.Preview, out.Artifact.Data); err != nil {
			log.Error().Err(err).Msg("failed to save artifact")
		}
	}
	return out.Answer, out.Meta, nil
}

// Chat Handler (supports both full JSON and live SSE token streaming)
func (h *Handler) handleChat(w http.ResponseWriter, r *http.Request) {
	var body chatBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "")
		return
	}
	text := strings.TrimSpace(body.Content)
	if text == "" && len(body.Files) == 0 && body.Action != rag.ActionExpert {
		writeError(w, http.StatusBadRequest, "empty", "")
		return
	}
	conversationID := body.ConversationID
	created := false
	if conversationID == "" {
		conversationID = uuid.NewString()
		created = true
		if _, err := h.store.CreateConversation(conversationID, conversationTitle(text, body.Files)); err != nil {
			log.Error().Err(err).Msg("failed to create conversation")
			writeError(w, http.StatusInternalServerError, "db_error", "")
			return
		}
	}
	conv, err := h.store.GetConversation(conversationID)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "")
		return
	}
	// Prepare user content
	userContent := text
	if userContent == "" {
		if len(body.Files) > 0 {
			var names []string
			for _, f := range body.Files {
				names = append(names, f.Name)
			}
			userContent = fmt.Sprintf("Проверьте документ: %s", strings.Join(names, ", "))
		} else {
			userContent = "Вопрос эксперту"
		}
	}
	// Save user message
	userMsgID := uuid.NewString()
	var userMeta any
	if len(body.Files) > 0 {
		metaFiles := make([]rag.AttachedFile, 0, len(body.Files))
		for _, f := range body.Files {
			metaFiles = append(metaFiles, rag.AttachedFile{Name: f.Name, Size: f.Size, Type: f.Type})
		}
		userMeta = map[string]any{"files": metaFiles}
	}
	userMsg, err := h.store.CreateMessage(userMsgID, conversationID, "user", userContent, userMeta)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", "")
		return
	}
	// Fetch history — only its length matters: a thread with a single message
	// is brand new and gets its title from this turn.
	historyMsgs, err := h.store.ListMessages(conversationID)
	if err != nil {
		historyMsgs = []store.Message{*userMsg}
	}
	firstTurn := len(historyMsgs) <= 1

	chatHistory := make([]rag.ChatMessage, 0, len(historyMsgs))
	for _, m := range historyMsgs {
		chatHistory = append(chatHistory, rag.ChatMessage{
			Role:    m.Role,
			Content: m.Content,
		})
	}

	// Check if client expects SSE streaming
	wantsSSE := strings.Contains(r.Header.Get("Accept"), "text/event-stream") || r.URL.Query().Get("stream") == "true"
	if wantsSSE {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")
		flusher, ok := w.(http.Flusher)
		if !ok {
			writeError(w, http.StatusInternalServerError, "streaming_unsupported", "Сервер не поддерживает потоковую отдачу")
			return
		}
		sendSSE := func(event string, payload any) {
			b, _ := json.Marshal(payload)
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, string(b))
			flusher.Flush()
		}
		// Send initial event immediately to flush HTTP headers and confirm connection
		sendSSE("start", map[string]any{
			"conversationId": conversationID,
			"userMessage":    userMsg,
		})
		tokenCallback := func(token string) {
			sendSSE("token", map[string]string{"token": token})
		}
		ans, meta, err := h.generateTurn(r, conv, userContent, body, chatHistory, tokenCallback)
		if r.Context().Err() != nil {
			// Client cancelled/aborted the request: keep whatever was produced,
			// but there is nobody left to send it to.
			if ans != "" {
				if _, err := h.store.CreateMessage(uuid.NewString(), conversationID, "assistant", ans, meta); err != nil {
					log.Error().Err(err).Msg("failed to persist the answer of a cancelled turn")
				}
			}
			return
		}
		if err != nil {
			log.Error().Err(err).Msg("chat turn failed")
			ans = chatFailureText
		}
		payload, err := h.turnPayload(conversationID, created, firstTurn, userContent, userMsg, ans, meta)
		if err != nil {
			// Headers are already flushed — the stream simply ends here.
			log.Error().Err(err).Msg("failed to persist the assistant answer")
			return
		}
		sendSSE("complete", payload)
		return
	}
	// Standard non-streaming JSON response
	ans, meta, err := h.generateTurn(r, conv, userContent, body, chatHistory, nil)
	if err != nil {
		log.Error().Err(err).Msg("chat turn failed")
		ans = chatFailureText
	}
	payload, err := h.turnPayload(conversationID, created, firstTurn, userContent, userMsg, ans, meta)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", "")
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

// chatFailureText is shown when a turn could not be answered at all.
const chatFailureText = "Произошла ошибка при формировании ответа. Пожалуйста, повторите запрос."

// turnPayload persists the assistant answer, titles a brand-new thread from its
// first message and builds the response body shared by the SSE and the plain
// JSON branches of POST /api/chat.
func (h *Handler) turnPayload(conversationID string, created, firstTurn bool, userContent string, userMsg *store.Message, answer string, meta any) (map[string]any, error) {
	asstMsg, err := h.store.CreateMessage(uuid.NewString(), conversationID, "assistant", answer, meta)
	if err != nil {
		return nil, err
	}
	var conv *store.Conversation
	if firstTurn {
		title := conversationTitle(userContent, nil)
		conv, err = h.store.UpdateConversation(conversationID, &title, nil, nil)
		if err != nil {
			log.Warn().Err(err).Msg("failed to retitle the conversation")
			conv = nil
		}
	}
	if conv == nil {
		if conv, err = h.store.GetConversation(conversationID); err != nil {
			return nil, err
		}
	}
	allMsgs, err := h.store.ListMessages(conversationID)
	if err != nil {
		allMsgs = []store.Message{}
	}
	return map[string]any{
		"conversation":     conv,
		"created":          created,
		"userMessage":      userMsg,
		"assistantMessage": asstMsg,
		"messages":         allMsgs,
	}, nil
}
// Conversation Handlers
func (h *Handler) handleListConversations(w http.ResponseWriter, r *http.Request) {
	list, err := h.store.ListConversations()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", "")
		return
	}
	writeJSON(w, http.StatusOK, list)
}
func (h *Handler) handleCreateConversation(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Title string `json:"title"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	conv, err := h.store.CreateConversation(uuid.NewString(), truncateTitleOr(body.Title, defaultConversationTitle))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", "")
		return
	}
	writeJSON(w, http.StatusOK, conv)
}
func (h *Handler) handleDeleteAllConversations(w http.ResponseWriter, r *http.Request) {
	if err := h.store.ClearAllConversations(); err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", "")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
func (h *Handler) handleGetConversation(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	conv, err := h.store.GetConversation(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "")
		return
	}
	msgs, err := h.store.ListMessages(id)
	if err != nil {
		msgs = []store.Message{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"conversation": conv,
		"messages":     msgs,
	})
}
func (h *Handler) handleUpdateConversation(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var body struct {
		Title  *string `json:"title"`
		Pinned *bool   `json:"pinned"`
		Draft  any     `json:"draft"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "")
		return
	}
	updated, err := h.store.UpdateConversation(id, body.Title, body.Pinned, body.Draft)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "")
		return
	}
	writeJSON(w, http.StatusOK, updated)
}
func (h *Handler) handleDeleteConversation(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.store.DeleteConversation(id); err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", "")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
// Settings Handlers
func (h *Handler) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	st, err := h.store.GetSettings()
	if err != nil {
		log.Warn().Err(err).Msg("settings load failed, serving defaults")
		st = &store.Setting{Theme: store.DefaultTheme, Language: store.DefaultLanguage}
	}
	writeJSON(w, http.StatusOK, st)
}
func (h *Handler) handleUpdateSettings(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Theme    string `json:"theme"`
		Language string `json:"language"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "")
		return
	}
	// A partial PATCH must not reset the other preference: fill the gaps from
	// the stored row (the UI sends only the field the user just changed).
	if body.Theme == "" || body.Language == "" {
		if current, err := h.store.GetSettings(); err == nil {
			if body.Theme == "" {
				body.Theme = current.Theme
			}
			if body.Language == "" {
				body.Language = current.Language
			}
		}
	}
	if body.Theme == "" {
		body.Theme = store.DefaultTheme
	}
	if body.Language == "" {
		body.Language = store.DefaultLanguage
	}
	st, err := h.store.UpdateSettings(body.Theme, body.Language)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", "")
		return
	}
	writeJSON(w, http.StatusOK, st)
}
// Templates Handlers
func (h *Handler) handleListTemplates(w http.ResponseWriter, r *http.Request) {
	tpls, err := h.store.ListTemplates()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", "")
		return
	}
	writeJSON(w, http.StatusOK, tpls)
}

// handleListModules GET /api/modules returns the modular document catalogue
// straight from the generator registry, so the builder form and the .docx
// generator can never drift apart.
func (h *Handler) handleListModules(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, docx.Modules())
}

// handleSaveModule POST /api/admin/modules allows dynamic registration of modules at runtime.
func (h *Handler) handleSaveModule(w http.ResponseWriter, r *http.Request) {
	var mod docx.ModuleDefinition
	if err := json.NewDecoder(r.Body).Decode(&mod); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	if mod.ID == "" || mod.Title == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "Поля 'id' и 'title' обязательны")
		return
	}
	docx.RegisterModule(mod)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "module": mod})
}

// handleAdminStats GET /api/admin/stats returns the live infrastructure numbers
// of the admin dashboard instead of the hard-coded ones it used to render.
func (h *Handler) handleAdminStats(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), adminStatsTimeout)
	defer cancel()

	regs, err := h.store.ListRegulations()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", "")
		return
	}
	active := 0
	for _, reg := range regs {
		if reg.Status == store.RegActive {
			active++
		}
	}
	stats := map[string]any{
		"modules":           len(docx.ModuleIDs()),
		"kbChunks":          h.dialog.CorpusSize(),
		"minRelevanceScore": h.cfg.MinRelevanceScore,
		"regulations":       map[string]any{"total": len(regs), "active": active},
	}
	if h.rag != nil {
		stats["qdrant"] = h.rag.CollectionStats(ctx)
		stats["ollama"] = h.rag.LLMStats(ctx)
	}
	writeJSON(w, http.StatusOK, stats)
}
// Regulations Handlers
func (h *Handler) handleGetRegulations(w http.ResponseWriter, r *http.Request) {
	sectionID := r.URL.Query().Get("section")
	id := r.URL.Query().Get("id")
	if sectionID != "" {
		for _, reg := range RegulationsData {
			for _, s := range reg.Sections {
				if s.ID == sectionID {
					writeJSON(w, http.StatusOK, map[string]any{
						"regulation": reg,
						"section":    s,
					})
					return
				}
			}
		}
		// Fallback: section ids produced by the local knowledge index
		// (od184…, recom…, matrix…) resolve through the dialog engine.
		if chunk, ok := h.dialog.Section(sectionID); ok {
			writeJSON(w, http.StatusOK, map[string]any{
				"regulation": map[string]any{
					"id":             "kb-" + chunk.DocumentTitle,
					"title":          chunk.DocumentTitle,
					"updatedAtLabel": chunk.UpdatedAt, // empty when the corpus carries no edition
					"intro":          "Фрагмент нормативного документа из локальной базы знаний SancireAI.",
					"sections": []map[string]any{
						{"id": chunk.ID, "clause": chunk.Clause, "title": chunk.Title, "text": chunk.Text},
					},
				},
				"section": map[string]any{"id": chunk.ID},
			})
			return
		}
		writeError(w, http.StatusNotFound, "not_found", "")
		return
	}
	if id != "" {
		for _, reg := range RegulationsData {
			if reg.ID == id {
				writeJSON(w, http.StatusOK, map[string]any{
					"regulation": reg,
				})
				return
			}
		}
		writeError(w, http.StatusNotFound, "not_found", "")
		return
	}
	var summaryList []map[string]any
	for _, reg := range RegulationsData {
		summaryList = append(summaryList, map[string]any{
			"id":             reg.ID,
			"title":          reg.Title,
			"shortTitle":     reg.ShortTitle,
			"updatedAt":      reg.UpdatedAt,
			"updatedAtLabel": reg.UpdatedAtLabel,
		})
	}
	writeJSON(w, http.StatusOK, summaryList)
}
// Level 2: Document Generation Handler
func (h *Handler) handleGenerateDocument(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ModuleID string            `json:"moduleId"`
		Values   map[string]string `json:"values"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "")
		return
	}
	docBytes, filename, err := docx.GenerateModuleDocx(h.cfg.TemplatesDir, body.ModuleID, body.Values)
	if err != nil {
		writeError(w, http.StatusBadRequest, "generation_failed", err.Error())
		return
	}
	writeDocx(w, filename, docBytes)
}
// Level 3: Document Audit Handler
func (h *Handler) handleAuditDocument(w http.ResponseWriter, r *http.Request) {
	// Parse multipart form with 15MB limit
	if err := r.ParseMultipartForm(docx.MaxDocxUploadSize); err != nil {
		writeError(w, http.StatusBadRequest, "file_too_large",
			fmt.Sprintf("Размер файла превышает %d МБ", sizeLimitMB(docx.MaxDocxUploadSize)))
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "missing_file", "Файл .docx не передан")
		return
	}
	defer file.Close()
	if !strings.HasSuffix(strings.ToLower(header.Filename), ".docx") {
		writeError(w, http.StatusBadRequest, "invalid_format", "Поддерживается только формат DOCX")
		return
	}
	data, err := io.ReadAll(file)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "read_failed", "")
		return
	}
	overrideModuleID := r.FormValue("moduleId")
	report, err := docx.AuditDocx(data, header.Filename, overrideModuleID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "audit_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, report)
}
// Admin Versions Handlers
func (h *Handler) handleListVersions(w http.ResponseWriter, r *http.Request) {
	list, err := h.store.ListVersions()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", "")
		return
	}
	writeJSON(w, http.StatusOK, list)
}
func (h *Handler) handleActivateVersion(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "")
		return
	}
	if err := h.store.ActivateVersion(body.Code); err != nil {
		writeError(w, http.StatusBadRequest, "activate_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "activeCode": body.Code})
}
// Health Handler
func (h *Handler) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"service": "SancireAI Backend",
		"version": serviceVersion,
		"qdrant":  h.cfg.QdrantURL,
		"ollama":  h.cfg.OllamaURL,
	})
}
// ---------------------------------------------------------------------------
// Admin: regulation lifecycle (upload → activate → archive)
// ---------------------------------------------------------------------------

const maxRegulationUpload = 30 << 20 // 30 MB

var regulationKinds = map[string]bool{
	"Инструкция": true, "Матрица визирования": true, "Методические рекомендации": true, "Другое": true,
}

// handleListRegulations GET /api/admin/regulations
func (h *Handler) handleListRegulations(w http.ResponseWriter, r *http.Request) {
	list, err := h.store.ListRegulations()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", "")
		return
	}
	if list == nil {
		list = []store.Regulation{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"regulations": list})
}

// handleUploadRegulation POST /api/admin/regulations/upload (multipart)
func (h *Handler) handleUploadRegulation(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(maxRegulationUpload); err != nil {
		writeError(w, http.StatusBadRequest, "file_too_large",
			fmt.Sprintf("Размер файла превышает %d МБ", sizeLimitMB(maxRegulationUpload)))
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "missing_file", "Файл не передан")
		return
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxRegulationUpload+1))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "read_failed", "")
		return
	}
	if len(data) > maxRegulationUpload {
		writeError(w, http.StatusBadRequest, "file_too_large",
			fmt.Sprintf("Размер файла превышает %d МБ", sizeLimitMB(maxRegulationUpload)))
		return
	}
	title := strings.TrimSpace(r.FormValue("title"))
	if title == "" {
		title = strings.TrimSuffix(header.Filename, filepath.Ext(header.Filename))
	}
	kind := strings.TrimSpace(r.FormValue("kind"))
	if kind == "" || !regulationKinds[kind] {
		kind = "Другое"
	}
	text, err := extract.Text(header.Filename, data)
	if err != nil {
		writeError(w, http.StatusBadRequest, "extract_failed", err.Error())
		return
	}
	pieces := extract.ChunkText(text)
	if len(pieces) == 0 {
		writeError(w, http.StatusBadRequest, "empty_text", "Не удалось выделить текстовые фрагменты")
		return
	}
	id := uuid.NewString()
	dir := filepath.Join(h.cfg.DataDir, "regulations")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		writeError(w, http.StatusInternalServerError, "fs_error", "")
		return
	}
	ext := strings.ToLower(filepath.Ext(header.Filename))
	filePath := filepath.Join(dir, id+ext)
	if err := os.WriteFile(filePath, data, 0o644); err != nil {
		writeError(w, http.StatusInternalServerError, "fs_error", "")
		return
	}
	chunks := make([]store.RegulationChunk, 0, len(pieces))
	for _, pc := range pieces {
		chunks = append(chunks, store.RegulationChunk{
			ID:           fmt.Sprintf("%s|c%d", id, pc.Seq),
			RegulationID: id,
			Seq:          pc.Seq,
			Clause:       pc.Clause,
			Title:        pc.Title,
			Text:         pc.Text,
		})
	}
	if err := h.store.SaveRegulation(id, title, kind, header.Filename, filePath, store.RegDraft, chunks); err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":          true,
		"regulation":  map[string]any{"id": id, "title": title, "kind": kind, "status": store.RegDraft, "chunkCount": len(chunks)},
		"message":     fmt.Sprintf("Загружено и нарезано на %d фрагментов. Документ в статусе «на модерации» — сделайте его активным, чтобы он участвовал в ответах.", len(chunks)),
	})
}

// handleRegulationStatus POST /api/admin/regulations/{id}/activate|archive
func (h *Handler) handleRegulationStatus(w http.ResponseWriter, r *http.Request, status string) {
	id := chi.URLParam(r, "id")
	if _, err := h.store.GetRegulation(id); err != nil {
		writeError(w, http.StatusNotFound, "not_found", "")
		return
	}
	prevIDs, targetIDs, err := h.store.SetRegulationStatus(id, status)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	h.refreshRegulationIndex()
	// Best-effort Qdrant sync (async — never blocks the admin action).
	// Убираем из индекса предыдущие активные редакции и, если документ
	// уходит в архив, — его собственные фрагменты (иначе архивная редакция
	// продолжает находиться в векторном поиске).
	removeIDs := append([]string{}, prevIDs...)
	if status != store.RegActive {
		removeIDs = append(removeIDs, targetIDs...)
	}
	if len(removeIDs) > 0 {
		ids := removeIDs
		h.asyncQdrant(qdrantRemoveTimeout, "points not removed",
			func(ctx context.Context) error { return h.rag.RemoveChunkPoints(ctx, ids) })
	}
	if status == store.RegActive && len(targetIDs) > 0 {
		rows, err := h.store.RegulationChunks(id)
		if err != nil {
			log.Warn().Err(err).Str("regulation", id).Msg("chunks for the qdrant push not loaded")
		} else {
			chunks := regulationToScored(rows)
			h.asyncQdrant(qdrantPushTimeout, "regulation push failed",
				func(ctx context.Context) error { return h.rag.PushChunks(ctx, chunks) })
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "status": status})
}

// handleActivateRegulation POST /api/admin/regulations/{id}/activate
func (h *Handler) handleActivateRegulation(w http.ResponseWriter, r *http.Request) {
	h.handleRegulationStatus(w, r, store.RegActive)
}

// handleArchiveRegulation POST /api/admin/regulations/{id}/archive
func (h *Handler) handleArchiveRegulation(w http.ResponseWriter, r *http.Request) {
	h.handleRegulationStatus(w, r, store.RegArchived)
}

// handleDeleteRegulation DELETE /api/admin/regulations/{id}
func (h *Handler) handleDeleteRegulation(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if _, err := h.store.GetRegulation(id); err != nil {
		writeError(w, http.StatusNotFound, "not_found", "")
		return
	}
	// The stored file path must be read BEFORE the row is deleted.
	filePath, pathErr := h.store.RegulationFilePath(id)
	chunkIDs, err := h.store.RegulationChunkIDs(id)
	if err != nil {
		log.Warn().Err(err).Str("regulation", id).Msg("chunk ids for vector cleanup not loaded")
	}
	if err := h.store.DeleteRegulation(id); err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", "")
		return
	}
	if pathErr != nil {
		log.Warn().Err(pathErr).Str("regulation", id).Msg("uploaded file path unknown, the file stays on disk")
	} else if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		log.Warn().Err(err).Str("path", filePath).Msg("uploaded regulation file not removed")
	}
	h.refreshRegulationIndex()
	// Векторы удаляем независимо от статуса: документ мог быть в архиве.
	if len(chunkIDs) > 0 {
		h.asyncQdrant(qdrantCleanupTimeout, "points of a deleted regulation not removed",
			func(ctx context.Context) error { return h.rag.RemoveChunkPoints(ctx, chunkIDs) })
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// asyncQdrant runs a vector-index update in the background with its own
// timeout. Qdrant being down must never fail an admin action: the local index
// stays the source of truth, so failures are only logged.
func (h *Handler) asyncQdrant(timeout time.Duration, what string, op func(ctx context.Context) error) {
	if h.rag == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		if err := op(ctx); err != nil {
			log.Warn().Err(err).Msg("qdrant: " + what)
		}
	}()
}

// handleDownloadRegulation GET /api/admin/regulations/{id}/file
func (h *Handler) handleDownloadRegulation(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	reg, err := h.store.GetRegulation(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "")
		return
	}
	filePath, err := h.store.RegulationFilePath(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "")
		return
	}
	w.Header().Set("Content-Disposition", contentDisposition(reg.Filename))
	http.ServeFile(w, r, filePath)
}

// handleGetArtifact serves a generated .docx back to the client.
func (h *Handler) handleGetArtifact(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	art, err := h.store.GetArtifact(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "")
		return
	}
	writeDocx(w, art.Filename, art.Data)
}

