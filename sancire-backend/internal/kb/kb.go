// Package kb provides the on-premise, deterministic knowledge index used by
// the chat assistant. It works without Ollama/Qdrant (100% local contour) and
// serves as a graceful fallback whenever the vector infrastructure is
// unavailable. The index is built from two corpus files:
//
//   - corpus/all_chunks.json     — curated, pre-cleaned sections;
//   - corpus/documents.json      — full page texts; pages that the curated
//     index does not cover (e.g. Глава 10 of the Instruction) are split into
//     clause-level chunks on the fly.
package kb

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"unicode/utf8"
)

// Chunk is one citable piece of a normative document.
type Chunk struct {
	ID            string `json:"id"`
	DocumentTitle string `json:"document_title"`
	Clause        string `json:"clause"`
	Title         string `json:"title"`
	Text          string `json:"text"`
	Page          int    `json:"page"`
	UpdatedAt     string `json:"updated_at"`
	Category      string `json:"category"`
	VersionID     string `json:"version_id"`

	compiled *compiled
}

// Scored is a search hit.
type Scored struct {
	Chunk *Chunk
	Score float64
}

// Index is the in-memory knowledge index.
type Index struct {
	mu sync.RWMutex
	// base = corpus files; regChunks = admin-uploaded active regulations.
	chunks    []*Chunk
	byID      map[string]*Chunk
	base      []*Chunk
	regChunks []*Chunk
}

var stopwords = map[string]bool{
	"и": true, "в": true, "во": true, "на": true, "с": true, "со": true, "по": true,
	"к": true, "о": true, "об": true, "от": true, "до": true, "за": true, "для": true,
	"это": true, "что": true, "как": true, "а": true, "но": true, "или": true, "ли": true,
	"же": true, "бы": true, "не": true, "ни": true, "вы": true, "мы": true, "я": true,
	"он": true, "она": true, "они": true, "чтобы": true, "когда": true,
	"где": true, "кто": true, "его": true, "ее": true, "её": true, "их": true,
	"мне": true, "мой": true, "моя": true, "при": true, "из": true, "у": true,
	"то": true, "есть": true, "можно": true, "нужно": true, "нужен": true,
	"нужна": true, "вот": true, "так": true, "такой": true, "какой": true,
	"какие": true, "чего": true, "чем": true, "ваш": true, "ваша": true,
	"the": true, "of": true, "and": true,
}

var (
	reNonWord   = regexp.MustCompile(`[^0-9a-zа-яё]+`)
	reClause    = regexp.MustCompile(`(\d{1,2}\.\d{1,2})\.\s`)
	reMultiSp   = regexp.MustCompile(`\s+`)
)

func normalize(s string) string {
	s = strings.ToLower(strings.ReplaceAll(s, "ё", "е"))
	s = reNonWord.ReplaceAllString(s, " ")
	return reMultiSp.ReplaceAllString(strings.TrimSpace(s), " ")
}

func tokenize(s string) []string {
	norm := normalize(s)
	if norm == "" {
		return nil
	}
	var out []string
	for _, w := range strings.Split(norm, " ") {
		if len(w) < 2 || stopwords[w] {
			continue
		}
		out = append(out, w)
	}
	return out
}

// matchKey collapses long words to their first 5 characters so that light
// Russian inflection ("согласование"/"согласования") still matches.
func matchKey(w string) string {
	r := []rune(w)
	if len(r) > 4 {
		return string(r[:4])
	}
	return w
}

type compiled struct {
	textKeys map[string]int
	titleSet map[string]bool
	normText string
}

func compileChunk(c *Chunk) *compiled {
	cp := &compiled{
		textKeys: map[string]int{},
		titleSet: map[string]bool{},
		normText: normalize(c.Text),
	}
	for _, w := range tokenize(c.Text) {
		cp.textKeys[matchKey(w)]++
	}
	for _, w := range tokenize(c.Title + " " + c.Clause + " " + c.DocumentTitle) {
		cp.titleSet[matchKey(w)] = true
	}
	return cp
}

// Load builds the index from the corpus directory.
func Load(corpusDir string) (*Index, error) {
	ix := &Index{byID: map[string]*Chunk{}}
	add := func(c *Chunk) {
		if c.ID == "" || c.Text == "" {
			return
		}
		if _, dup := ix.byID[c.ID]; dup {
			return
		}
		c.compiled = compileChunk(c)
		ix.byID[c.ID] = c
		ix.chunks = append(ix.chunks, c)
	}

	// 1. Curated chunks.
	raw, err := os.ReadFile(filepath.Join(corpusDir, "all_chunks.json"))
	if err != nil {
		return nil, fmt.Errorf("read all_chunks.json: %w", err)
	}
	var curated []Chunk
	if err := json.Unmarshal(raw, &curated); err != nil {
		return nil, fmt.Errorf("parse all_chunks.json: %w", err)
	}
	coveredPages := map[int]bool{}
	for i := range curated {
		c := curated[i]
		if strings.HasPrefix(c.ID, "od184-") && c.Page > 0 {
			coveredPages[c.Page] = true
		}
		add(&c)
	}

	// 2. Clause-level chunks for instruction pages the curated index misses.
	docsRaw, err := os.ReadFile(filepath.Join(corpusDir, "documents.json"))
	if err == nil {
		var docs []struct {
			ID    string `json:"id"`
			Title string `json:"title"`
			Pages []struct {
				Page int    `json:"page"`
				Text string `json:"text"`
			} `json:"pages"`
			TotalPages int `json:"total_pages"`
		}
		if err := json.Unmarshal(docsRaw, &docs); err == nil {
			for _, d := range docs {
				if d.ID != "dou_instruction" {
					continue
				}
				for _, pg := range d.Pages {
					if coveredPages[pg.Page] {
						continue
					}
					for _, c := range splitPageIntoClauses(pg.Page, pg.Text, d.Title) {
						add(c)
					}
				}
			}
		}
	}

	// Snapshot the corpus: the immutable base that every SetRegulations
	// rebuild merges the active regulation chunks into.
	ix.base = append([]*Chunk(nil), ix.chunks...)
	return ix, nil
}

// genuine abbreviations that look like watermark fragments but are real terms.
var abbrevWhitelist = map[string]bool{
	"ДОУ": true, "ИС": true, "ИТ": true, "РО": true, "СП": true, "ЦА": true,
	"ПД": true, "ЭП": true, "ОД": true, "АП": true, "СФ": true, "ГД": true,
	"НД": true, "СЭД": true, "ИИС": true, "РК": true, "ТЗ": true, "БР": true,
	"ЦБ": true, "ГОС": true, "СМ": true, "ПП": true, "АС": true, "ФЗ": true,
}

// cleanWatermark removes the diagonal "УЧЕБНЫЙ ПРИМЕР…" stamp fragments that
// the PDF extractor interleaves into the raw page text. Fragments are short
// all-caps tokens ("УЧ ЕБ НЫ Й", "ДЕ ЙС ТВ", "ОМ") that appear in runs;
// genuine abbreviations (ДОУ, ИС, ИТ …) are preserved.
func cleanWatermark(text string) string {
	text = reMultiSp.ReplaceAllString(strings.TrimSpace(text), " ")
	words := strings.Split(text, " ")
	isFrag := func(w string) bool {
		core := strings.TrimLeft(w, ".·-")
		if core == "" || !isAllCapsCyrillic(core) {
			return false
		}
		if abbrevWhitelist[core] {
			return false
		}
		return utf8.RuneCountInString(core) <= 4
	}
	var out []string
	i := 0
	for i < len(words) {
		if !isFrag(words[i]) {
			out = append(out, words[i])
			i++
			continue
		}
		// Measure the maximal run of fragment tokens.
		j := i
		for j < len(words) && isFrag(words[j]) {
			j++
		}
		runLen := j - i
		singleShort := runLen == 1 && utf8.RuneCountInString(strings.TrimLeft(words[i], ".·-")) <= 3
		if runLen >= 2 || singleShort {
			// drop the whole run (watermark fragment)
			if len(out) > 0 && j < len(words) {
				// keep spacing stable; collapse later
			}
		} else {
			out = append(out, words[i:j]...)
		}
		i = j
	}
	return reMultiSp.ReplaceAllString(strings.Join(out, " "), " ")
}

func isAllCapsCyrillic(s string) bool {
	for _, r := range s {
		if r < 'А' || r > 'Я' {
			if r == 'Ё' {
				continue
			}
			return false
		}
	}
	return len(s) > 0
}

// splitPageIntoClauses converts a raw instruction page into clause chunks.
func splitPageIntoClauses(page int, text, docTitle string) []*Chunk {
	clean := cleanWatermark(text)
	clean = strings.TrimSpace(clean)
	if clean == "" {
		return nil
	}
	type piece struct{ clause, body string }
	var pieces []piece
	matches := reClause.FindAllStringSubmatchIndex(clean, -1)
	if len(matches) == 0 {
		pieces = append(pieces, piece{clause: fmt.Sprintf("Стр. %d", page), body: clean})
	} else {
		if matches[0][0] > 0 {
			pieces = append(pieces, piece{clause: fmt.Sprintf("Стр. %d", page), body: clean[:matches[0][0]]})
		}
		for i, m := range matches {
			start := m[0]
			end := len(clean)
			if i+1 < len(matches) {
				end = matches[i+1][0]
			}
			clause := clean[m[2]:m[3]]
			body := strings.TrimSpace(clean[start:end])
			pieces = append(pieces, piece{clause: "п. " + clause, body: body})
		}
	}
	var out []*Chunk
	for _, pc := range pieces {
		body := strings.TrimSpace(pc.body)
		if len(body) < 40 {
			continue
		}
		id := fmt.Sprintf("od184-p%d|%s", page, strings.ReplaceAll(pc.clause, "п. ", ""))
		if strings.HasPrefix(pc.clause, "Стр.") {
			id = fmt.Sprintf("od184-p%d|pre", page)
		}
		out = append(out, &Chunk{
			ID:            id,
			DocumentTitle: docTitle,
			Clause:        pc.clause,
			Title:         fmt.Sprintf("Инструкция по ДОУ, стр. %d", page),
			Text:          body,
			Page:          page,
			UpdatedAt:     "15.10.2026",
			Category:      "instruction",
			VersionID:     "v1.0",
		})
	}
	return out
}

// SetRegulations replaces the admin-uploaded regulation slice and rebuilds
// the merged view. Safe for concurrent use with Search/Get.
func (ix *Index) SetRegulations(chunks []*Chunk) {
	if ix == nil {
		return
	}
	ix.mu.Lock()
	defer ix.mu.Unlock()
	ix.regChunks = nil
	ix.chunks = make([]*Chunk, 0, len(ix.base)+len(chunks))
	ix.byID = make(map[string]*Chunk, len(ix.base)+len(chunks))
	addMerged := func(c *Chunk) {
		if c == nil || c.ID == "" || c.Text == "" {
			return
		}
		if _, dup := ix.byID[c.ID]; dup {
			return
		}
		if c.compiled == nil {
			c.compiled = compileChunk(c)
		}
		ix.byID[c.ID] = c
		ix.chunks = append(ix.chunks, c)
	}
	for _, c := range ix.base {
		addMerged(c)
	}
	for _, c := range chunks {
		addMerged(c)
		ix.regChunks = append(ix.regChunks, c)
	}
}

// Get returns a chunk by its id.
func (ix *Index) Get(id string) (*Chunk, bool) {
	if ix == nil {
		return nil, false
	}
	ix.mu.RLock()
	defer ix.mu.RUnlock()
	c, ok := ix.byID[id]
	return c, ok
}

// Len reports how many chunks are indexed.
func (ix *Index) Len() int {
	if ix == nil {
		return 0
	}
	ix.mu.RLock()
	defer ix.mu.RUnlock()
	return len(ix.chunks)
}

// AllIDs returns every indexed chunk id (base + regulations).
func (ix *Index) AllIDs() []string {
	if ix == nil {
		return nil
	}
	ix.mu.RLock()
	defer ix.mu.RUnlock()
	ids := make([]string, 0, len(ix.chunks))
	for _, c := range ix.chunks {
		ids = append(ids, c.ID)
	}
	return ids
}

// Search ranks chunks for a natural-language query.
func (ix *Index) Search(query string, limit int) []Scored {
	if ix == nil {
		return nil
	}
	ix.mu.RLock()
	defer ix.mu.RUnlock()
	qTokens := tokenize(query)
	if len(qTokens) == 0 {
		return nil
	}
	// Deduplicate query keys.
	keyWeights := map[string]float64{}
	for _, w := range qTokens {
		key := matchKey(w)
		if len(w) >= 6 {
			keyWeights[key] += 1.0
		} else {
			keyWeights[key] += 1.0
		}
	}
	normQuery := normalize(query)
	scores := make([]Scored, 0, 16)
	for _, c := range ix.chunks {
		cp := c.compiled
		if cp == nil {
			continue
		}
		var score float64
		distinct := 0
		strongSingle := false
		for key, weight := range keyWeights {
			matched := false
			if cp.titleSet[key] {
				score += 3 * weight
				matched = true
				strongSingle = true
			}
			if n := cp.textKeys[key]; n > 0 {
				if n > 3 {
					n = 3
				}
				score += float64(n) * weight
				matched = true
				if n >= 2 {
					strongSingle = true
				}
			}
			if matched {
				distinct++
			}
		}
		phrase := len(normQuery) > 12 && strings.Contains(cp.normText, normQuery)
		if phrase {
			score += 6
		}
		// A single generic term is too weak a basis for a normative answer —
		// require either two distinct term hits, one strong hit (title or the
		// term repeated in the text) or an exact phrase match.
		if score > 0 && (distinct >= 2 || phrase || (distinct == 1 && strongSingle)) {
			scores = append(scores, Scored{Chunk: c, Score: score})
		}
	}
	sort.Slice(scores, func(i, j int) bool {
		if scores[i].Score != scores[j].Score {
			return scores[i].Score > scores[j].Score
		}
		return scores[i].Chunk.ID < scores[j].Chunk.ID
	})
	if limit > 0 && len(scores) > limit {
		scores = scores[:limit]
	}
	return scores
}

// Excerpt trims a chunk text for display inside chat citations.
func (c *Chunk) Excerpt(max int) string {
	text := reMultiSp.ReplaceAllString(c.Text, " ")
	runes := []rune(text)
	if max <= 0 || len(runes) <= max {
		return text
	}
	cut := string(runes[:max])
	if i := strings.LastIndex(cut, ". "); i > max/2 {
		return cut[:i+1]
	}
	return cut + "…"
}

// SentenceCount is a tiny helper for answer composition.
func SentenceCount(s string) int {
	return strings.Count(s, ".") + strings.Count(s, "!")
}
