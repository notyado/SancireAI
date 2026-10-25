package api
import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
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
type Handler struct {
	cfg    config.Config
	store  *store.Store
	rag    *rag.Engine
	dialog *dialog.Engine
}
func NewHandler(cfg config.Config, s *store.Store, r *rag.Engine) *Handler {
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
// Auth Handlers
func (h *Handler) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid_json"}`, http.StatusBadRequest)
		return
	}
	user, err := h.store.ValidateUser(strings.TrimSpace(req.Username), req.Password)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid_credentials", "message": err.Error()})
		return
	}
	token, err := GenerateToken(user, h.cfg.JWTSecret, h.cfg.TokenTTL)
	if err != nil {
		http.Error(w, `{"error":"token_generation_failed"}`, http.StatusInternalServerError)
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
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
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

// generateTurn routes one chat turn either to the deterministic dialog engine
// (wizard / knowledge / audit) or to the legacy RAG pipeline for explain and
// expert actions. It persists wizard draft state and generated artifacts.
func (h *Handler) generateTurn(r *http.Request, conv *store.Conversation, userContent string, history []rag.ChatMessage, body chatBody, versionCode string, token func(string)) (string, any, error) {
	if body.Action == "explain" || body.Action == "expert" {
		return h.rag.GenerateAnswer(r.Context(), userContent, history, body.Action, body.Selection, versionCode, token)
	}
	// "Уточнить" deep-dives the selected fragment: query the KB with the
	// selection itself so the answer stays grounded in that passage.
	dText := userContent
	if body.Action == "clarify" && body.Selection != "" {
		dText = body.Selection
	}
	out := h.dialog.Respond(dialog.Request{
		Ctx:    r.Context(),
		Text:   dText,
		Action: body.Action,
		Files:  body.Files,
		Draft:  conv.Draft,
		Token:  token,
	})
	if out.Artifact != nil {
		id := uuid.NewString()
		if err := h.store.SaveArtifact(id, out.Artifact.FileName, out.Artifact.Title, out.Artifact.Kind, out.Artifact.Preview, out.Artifact.Data); err != nil {
			log.Error().Err(err).Msg("failed to save artifact")
		} else if out.Meta.Document != nil {
			out.Meta.Document.ArtifactID = id
		}
	}
	if out.ChangeDraft {
		var draft any = out.NewDraft
		if out.ClearDraft {
			// Explicit JSON null clears the draft; nil would mean "keep".
			draft = json.RawMessage("null")
		}
		if _, err := h.store.UpdateConversation(conv.ID, nil, nil, draft); err != nil {
			log.Error().Err(err).Msg("failed to persist wizard draft")
		}
	}
	return out.Answer, out.Meta, nil
}

// Chat Handler (supports both full JSON and live SSE token streaming)
func (h *Handler) handleChat(w http.ResponseWriter, r *http.Request) {
	var body chatBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"invalid_json"}`, http.StatusBadRequest)
		return
	}
	text := strings.TrimSpace(body.Content)
	if text == "" && len(body.Files) == 0 && body.Action != "expert" {
		http.Error(w, `{"error":"empty"}`, http.StatusBadRequest)
		return
	}
	conversationID := body.ConversationID
	created := false
	if conversationID == "" {
		conversationID = uuid.NewString()
		created = true
		title := truncateTitle(text)
		if title == "" && len(body.Files) > 0 {
			title = body.Files[0].Name
		}
		if title == "" {
			title = "Новый диалог"
		}
		if _, err := h.store.CreateConversation(conversationID, title); err != nil {
			log.Error().Err(err).Msg("failed to create conversation")
			http.Error(w, `{"error":"db_error"}`, http.StatusInternalServerError)
			return
		}
	}
	conv, err := h.store.GetConversation(conversationID)
	if err != nil {
		http.Error(w, `{"error":"not_found"}`, http.StatusNotFound)
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
		http.Error(w, `{"error":"db_error"}`, http.StatusInternalServerError)
		return
	}
	// Fetch history
	historyMsgs, err := h.store.ListMessages(conversationID)
	if err != nil {
		historyMsgs = []store.Message{*userMsg}
	}
	var chatHistory []rag.ChatMessage
	for _, m := range historyMsgs {
		chatHistory = append(chatHistory, rag.ChatMessage{Role: m.Role, Content: m.Content})
	}
	// Active version
	activeVer, _ := h.store.GetActiveVersion()
	versionCode := "v1.0"
	if activeVer != nil {
		versionCode = activeVer.Code
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
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
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
		ans, meta, err := h.generateTurn(r, conv, userContent, chatHistory, body, versionCode, tokenCallback)
		if r.Context().Err() != nil {
			// Client cancelled/aborted request
			if ans != "" {
				asstMsgID := uuid.NewString()
				h.store.CreateMessage(asstMsgID, conversationID, "assistant", ans, meta)
			}
			return
		}
		if err != nil {
			ans = "Произошла ошибка при формировании ответа. Пожалуйста, повторите запрос."
		}
		asstMsgID := uuid.NewString()
		asstMsg, _ := h.store.CreateMessage(asstMsgID, conversationID, "assistant", ans, meta)
		// Update title if first message
		if len(historyMsgs) <= 1 {
			title := truncateTitle(userContent)
			conv, _ = h.store.UpdateConversation(conversationID, &title, nil, nil)
		} else {
			conv, _ = h.store.GetConversation(conversationID)
		}
		allMsgs, _ := h.store.ListMessages(conversationID)
		sendSSE("complete", map[string]any{
			"conversation":     conv,
			"created":          created,
			"userMessage":      userMsg,
			"assistantMessage": asstMsg,
			"messages":         allMsgs,
		})
		return
	}
	// Standard non-streaming JSON response
	ans, meta, err := h.generateTurn(r, conv, userContent, chatHistory, body, versionCode, nil)
	if err != nil {
		ans = "Произошла ошибка при обращении к базе знаний. Пожалуйста, повторите попытку."
	}
	asstMsgID := uuid.NewString()
	asstMsg, err := h.store.CreateMessage(asstMsgID, conversationID, "assistant", ans, meta)
	if err != nil {
		http.Error(w, `{"error":"db_error"}`, http.StatusInternalServerError)
		return
	}
	if len(historyMsgs) <= 1 {
		title := truncateTitle(userContent)
		conv, _ = h.store.UpdateConversation(conversationID, &title, nil, nil)
	} else {
		conv, _ = h.store.GetConversation(conversationID)
	}
	allMsgs, _ := h.store.ListMessages(conversationID)
	writeJSON(w, http.StatusOK, map[string]any{
		"conversation":     conv,
		"created":          created,
		"userMessage":      userMsg,
		"assistantMessage": asstMsg,
		"messages":         allMsgs,
	})
}
// Conversation Handlers
func (h *Handler) handleListConversations(w http.ResponseWriter, r *http.Request) {
	list, err := h.store.ListConversations()
	if err != nil {
		http.Error(w, `{"error":"db_error"}`, http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, list)
}
func (h *Handler) handleCreateConversation(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Title string `json:"title"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	title := body.Title
	if title == "" {
		title = "Новый диалог"
	}
	conv, err := h.store.CreateConversation(uuid.NewString(), title)
	if err != nil {
		http.Error(w, `{"error":"db_error"}`, http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, conv)
}
func (h *Handler) handleDeleteAllConversations(w http.ResponseWriter, r *http.Request) {
	if err := h.store.ClearAllConversations(); err != nil {
		http.Error(w, `{"error":"db_error"}`, http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
func (h *Handler) handleGetConversation(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	conv, err := h.store.GetConversation(id)
	if err != nil {
		http.Error(w, `{"error":"not_found"}`, http.StatusNotFound)
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
		http.Error(w, `{"error":"invalid_json"}`, http.StatusBadRequest)
		return
	}
	updated, err := h.store.UpdateConversation(id, body.Title, body.Pinned, body.Draft)
	if err != nil {
		http.Error(w, `{"error":"not_found"}`, http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}
func (h *Handler) handleDeleteConversation(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.store.DeleteConversation(id); err != nil {
		http.Error(w, `{"error":"db_error"}`, http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
// Settings Handlers
func (h *Handler) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	st, err := h.store.GetSettings()
	if err != nil {
		st = &store.Setting{ID: "default", Theme: "dark", Language: "ru"}
	}
	writeJSON(w, http.StatusOK, st)
}
func (h *Handler) handleUpdateSettings(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Theme    string `json:"theme"`
		Language string `json:"language"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"invalid_json"}`, http.StatusBadRequest)
		return
	}
	if body.Theme == "" {
		body.Theme = "dark"
	}
	if body.Language == "" {
		body.Language = "ru"
	}
	st, err := h.store.UpdateSettings(body.Theme, body.Language)
	if err != nil {
		http.Error(w, `{"error":"db_error"}`, http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, st)
}
// Templates Handlers
func (h *Handler) handleListTemplates(w http.ResponseWriter, r *http.Request) {
	tpls, err := h.store.ListTemplates()
	if err != nil {
		http.Error(w, `{"error":"db_error"}`, http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, tpls)
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
		if chunkID, docTitle, clause, text, updatedAt, ok := h.dialog.Section(sectionID); ok {
			writeJSON(w, http.StatusOK, map[string]any{
				"regulation": map[string]any{
					"id":             "kb-" + docTitle,
					"title":          docTitle,
					"updatedAtLabel": firstNonEmpty(updatedAt, "15.10.2026"),
					"intro":          "Фрагмент нормативного документа из локальной базы знаний SancireAI.",
					"sections": []map[string]any{
						{"id": chunkID, "clause": clause, "title": "", "text": text},
					},
				},
				"section": map[string]any{"id": chunkID},
			})
			return
		}
		http.Error(w, `{"error":"not_found"}`, http.StatusNotFound)
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
		http.Error(w, `{"error":"not_found"}`, http.StatusNotFound)
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
		http.Error(w, `{"error":"invalid_json"}`, http.StatusBadRequest)
		return
	}
	docBytes, filename, err := docx.GenerateModuleDocx(h.cfg.TemplatesDir, body.ModuleID, body.Values)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "generation_failed", "message": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.wordprocessingml.document")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(docBytes)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(docBytes)
}
// Level 3: Document Audit Handler
func (h *Handler) handleAuditDocument(w http.ResponseWriter, r *http.Request) {
	// Parse multipart form with 15MB limit
	if err := r.ParseMultipartForm(docx.MaxDocxUploadSize); err != nil {
		http.Error(w, `{"error":"file_too_large","message":"Размер файла превышает 15 МБ"}`, http.StatusBadRequest)
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, `{"error":"missing_file","message":"Файл .docx не передан"}`, http.StatusBadRequest)
		return
	}
	defer file.Close()
	if !strings.HasSuffix(strings.ToLower(header.Filename), ".docx") {
		http.Error(w, `{"error":"invalid_format","message":"Поддерживается только формат DOCX"}`, http.StatusBadRequest)
		return
	}
	data, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, `{"error":"read_failed"}`, http.StatusInternalServerError)
		return
	}
	overrideModuleID := r.FormValue("moduleId")
	report, err := docx.AuditDocx(data, header.Filename, overrideModuleID)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "audit_failed", "message": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, report)
}
// Admin Versions Handlers
func (h *Handler) handleListVersions(w http.ResponseWriter, r *http.Request) {
	list, err := h.store.ListVersions()
	if err != nil {
		http.Error(w, `{"error":"db_error"}`, http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, list)
}
func (h *Handler) handleActivateVersion(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"invalid_json"}`, http.StatusBadRequest)
		return
	}
	if err := h.store.ActivateVersion(body.Code); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"activate_failed","message":"%s"}`, err.Error()), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "activeCode": body.Code})
}
// Health Handler
func (h *Handler) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"service": "SancireAI Backend",
		"version": "1.0.0",
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
		http.Error(w, `{"error":"db_error"}`, http.StatusInternalServerError)
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
		http.Error(w, `{"error":"file_too_large","message":"Размер файла превышает 30 МБ"}`, http.StatusBadRequest)
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, `{"error":"missing_file","message":"Файл не передан"}`, http.StatusBadRequest)
		return
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxRegulationUpload+1))
	if err != nil {
		http.Error(w, `{"error":"read_failed"}`, http.StatusInternalServerError)
		return
	}
	if len(data) > maxRegulationUpload {
		http.Error(w, `{"error":"file_too_large","message":"Размер файла превышает 30 МБ"}`, http.StatusBadRequest)
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
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "extract_failed", "message": err.Error()})
		return
	}
	pieces := extract.ChunkText(text)
	if len(pieces) == 0 {
		http.Error(w, `{"error":"empty_text","message":"Не удалось выделить текстовые фрагменты"}`, http.StatusBadRequest)
		return
	}
	id := uuid.NewString()
	dir := filepath.Join(h.cfg.DataDir, "regulations")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		http.Error(w, `{"error":"fs_error"}`, http.StatusInternalServerError)
		return
	}
	ext := strings.ToLower(filepath.Ext(header.Filename))
	filePath := filepath.Join(dir, id+ext)
	if err := os.WriteFile(filePath, data, 0o644); err != nil {
		http.Error(w, `{"error":"fs_error"}`, http.StatusInternalServerError)
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
		http.Error(w, `{"error":"db_error","message":"`+err.Error()+`"}`, http.StatusInternalServerError)
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
		http.Error(w, `{"error":"not_found"}`, http.StatusNotFound)
		return
	}
	prevIDs, targetIDs, err := h.store.SetRegulationStatus(id, status)
	if err != nil {
		http.Error(w, `{"error":"db_error","message":"`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}
	h.refreshRegulationIndex()
	// Best-effort Qdrant sync (async — never blocks the admin action).
	if h.rag != nil {
		ragEngine := h.rag
		if len(prevIDs) > 0 {
			ids := prevIDs
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
				defer cancel()
				if err := ragEngine.RemoveChunkPoints(ctx, ids); err != nil {
					log.Warn().Err(err).Msg("qdrant: prev active points not removed")
				}
			}()
		}
		if status == store.RegActive && len(targetIDs) > 0 {
			rows, err := h.store.RegulationChunks(id)
			if err == nil {
				chunks := regulationToScored(rows)
				go func() {
					ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
					defer cancel()
					if err := ragEngine.PushChunks(ctx, chunks); err != nil {
						log.Warn().Err(err).Msg("qdrant: regulation push failed")
					}
				}()
			}
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
	reg, err := h.store.GetRegulation(id)
	if err != nil {
		http.Error(w, `{"error":"not_found"}`, http.StatusNotFound)
		return
	}
	chunkIDs, _ := h.store.RegulationChunkIDs(id)
	if err := h.store.DeleteRegulation(id); err != nil {
		http.Error(w, `{"error":"db_error"}`, http.StatusInternalServerError)
		return
	}
	if fp, err := h.store.RegulationFilePath(id); err == nil {
		_ = fp
	}
	// file path lookup after delete fails — capture before delete next time;
	// best effort: glob the known filename pattern.
	_ = os.Remove(filepath.Join(h.cfg.DataDir, "regulations", id+".pdf"))
	_ = os.Remove(filepath.Join(h.cfg.DataDir, "regulations", id+".docx"))
	_ = os.Remove(filepath.Join(h.cfg.DataDir, "regulations", id+".txt"))
	_ = os.Remove(filepath.Join(h.cfg.DataDir, "regulations", id+".md"))
	h.refreshRegulationIndex()
	if h.rag != nil && reg.Status == store.RegActive && len(chunkIDs) > 0 {
		ragEngine := h.rag
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			_ = ragEngine.RemoveChunkPoints(ctx, chunkIDs)
		}()
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleDownloadRegulation GET /api/admin/regulations/{id}/file
func (h *Handler) handleDownloadRegulation(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	reg, err := h.store.GetRegulation(id)
	if err != nil {
		http.Error(w, `{"error":"not_found"}`, http.StatusNotFound)
		return
	}
	filePath, err := h.store.RegulationFilePath(id)
	if err != nil {
		http.Error(w, `{"error":"not_found"}`, http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename*=UTF-8''%s`, url.PathEscape(reg.Filename)))
	http.ServeFile(w, r, filePath)
}

// handleGetArtifact serves a generated .docx back to the client.
func (h *Handler) handleGetArtifact(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	art, err := h.store.GetArtifact(id)
	if err != nil {
		http.Error(w, `{"error":"not_found"}`, http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.wordprocessingml.document")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename*=UTF-8''%s`, url.PathEscape(art.Filename)))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(art.Data)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(art.Data)
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}
func truncateTitle(text string) string {
	s := strings.TrimSpace(text)
	if s == "" {
		return "Новый диалог"
	}
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.Join(strings.Fields(s), " ")
	runes := []rune(s)
	if len(runes) > 52 {
		return string(runes[:52]) + "…"
	}
	return s
}
