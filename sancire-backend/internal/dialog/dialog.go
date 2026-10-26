package dialog

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
	"sancireai/backend/internal/docx"
	"sancireai/backend/internal/kb"
	"sancireai/backend/internal/rag"
)

// File is an attachment forwarded from the chat client.
type File struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
	Type string `json:"type"`
	Data string `json:"data,omitempty"` // base64 payload (used for .docx audits)
}

// Request is one inbound chat turn.
type Request struct {
	Ctx     context.Context
	Text    string
	Files   []File
	Draft   any
	Token   func(string)
	History []rag.ChatMessage
}

// ArtifactOut is a generated file that the handler can persist.
type ArtifactOut struct {
	Data     []byte
	FileName string
	Title    string
	Kind     string
	Preview  string
}

// Outcome is the result of one chat turn.
type Outcome struct {
	Answer      string
	Meta        Meta
	NewDraft    any
	ChangeDraft bool
	ClearDraft  bool
	Artifact    *ArtifactOut
}

// Meta mirrors the JSON metadata attached to assistant messages.
type Meta struct {
	Intent   string            `json:"intent,omitempty"`
	Sources  []Source          `json:"sources,omitempty"`
	Audit    *docx.AuditReport `json:"audit,omitempty"`
	NotFound bool              `json:"notFound,omitempty"`
}

// Source points at a normative document section.
type Source struct {
	ID            string `json:"id"`
	DocumentTitle string `json:"documentTitle"`
	Clause        string `json:"clause"`
	Excerpt       string `json:"excerpt"`
	UpdatedAt     string `json:"updatedAt"`
}

// Engine answers chat turns using RAG and KB.
type Engine struct {
	ix           *kb.Index
	templatesDir string
	llm          *rag.Engine
}

func (e *Engine) UseLLM(r *rag.Engine) { e.llm = r }

func (e *Engine) ReloadRegulations(chunks []*kb.Chunk) {
	if e.ix != nil {
		e.ix.SetRegulations(chunks)
		log.Info().Int("chunks", len(chunks)).Msg("kb regulations reloaded")
	}
}

func NewEngine(templatesDir, corpusDir string) *Engine {
	ix, err := kb.Load(corpusDir)
	if err != nil {
		log.Warn().Err(err).Msg("kb index not loaded")
	}
	return &Engine{ix: ix, templatesDir: templatesDir}
}

func (e *Engine) Section(id string) (kb.Chunk, bool) {
	if e.ix == nil {
		return kb.Chunk{}, false
	}
	c, ok := e.ix.Get(id)
	if !ok || c == nil {
		return kb.Chunk{}, false
	}
	return *c, true
}

func (e *Engine) CorpusSize() int {
	if e.ix == nil {
		return 0
	}
	return e.ix.Len()
}

// Respond processes one chat turn directly via AI and knowledge base.
func (e *Engine) Respond(req Request) Outcome {
	text := strings.TrimSpace(req.Text)
	norm := normalizeText(text)

	// 1. Audit whenever a .docx payload arrives.
	if out, ok := e.tryAuditFiles(req, norm); ok {
		return out
	}

	// 2. Knowledge-base and conversational answer from AI / KB.
	return e.compose(req, text, norm)
}

func (e *Engine) reply(req Request, answer string, meta Meta) Outcome {
	emit(req.Token, answer)
	return Outcome{Answer: answer, Meta: meta}
}

func emit(token func(string), text string) {
	if token == nil || text == "" {
		return
	}
	runes := []rune(text)
	const chunkSize = 24
	for s := 0; s < len(runes); s += chunkSize {
		end := s + chunkSize
		if end > len(runes) {
			end = len(runes)
		}
		token(string(runes[s:end]))
		if end < len(runes) {
			time.Sleep(8 * time.Millisecond)
		}
	}
}

func (e *Engine) tryAuditFiles(req Request, norm string) (Outcome, bool) {
	for _, f := range req.Files {
		if !strings.HasSuffix(strings.ToLower(f.Name), ".docx") || f.Data == "" {
			continue
		}
		data, err := base64.StdEncoding.DecodeString(f.Data)
		if err != nil {
			return e.reply(req, "Не удалось прочитать файл — повреждённая кодировка. Попробуйте загрузить документ заново.", Meta{Intent: "audit.error"}), true
		}
		report, err := docx.AuditDocx(data, f.Name, "")
		if err != nil {
			return e.reply(req, fmt.Sprintf("Не удалось разобрать файл `%s`: %s\n\nПроверьте, что это корректный .docx.", f.Name, err.Error()), Meta{Intent: "audit.error", NotFound: true}), true
		}
		answer := auditAnswer(report)
		return e.reply(req, answer, Meta{
			Intent: "audit.report",
			Audit:  report,
		}), true
	}
	return Outcome{}, false
}

func normalizeText(s string) string {
	return strings.ToLower(strings.ReplaceAll(s, "ё", "е"))
}
