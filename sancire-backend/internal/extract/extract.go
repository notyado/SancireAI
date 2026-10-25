// Package extract pulls plain text out of uploaded regulation files
// (PDF / DOCX / TXT / MD) so they can be chunked into the knowledge index.
package extract

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/ledongthuc/pdf"
)

// Text returns the plain text of an uploaded document.
func Text(filename string, data []byte) (string, error) {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".txt", ".md", ".markdown", ".csv":
		s := string(data)
		s = strings.TrimPrefix(s, "\uFEFF")
		return s, nil
	case ".docx":
		return fromDOCX(data)
	case ".pdf":
		return fromPDF(data)
	default:
		return "", fmt.Errorf("формат %q не поддерживается — загрузите PDF, DOCX, TXT или MD", ext)
	}
}

var (
	reXMLTag  = regexp.MustCompile(`<[^>]+>`)
	reXMLSpace = regexp.MustCompile(`\r?\n\s*\r?\n+`)
)

func fromDOCX(data []byte) (string, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", fmt.Errorf("не похоже на DOCX: %w", err)
	}
	var docXML []byte
	for _, f := range zr.File {
		if f.Name == "word/document.xml" {
			rc, err := f.Open()
			if err != nil {
				return "", err
			}
			docXML, err = io.ReadAll(rc)
			rc.Close()
			if err != nil {
				return "", err
			}
			break
		}
	}
	if docXML == nil {
		return "", fmt.Errorf("в архиве нет word/document.xml")
	}
	s := string(docXML)
	s = strings.ReplaceAll(s, "</w:p>", "\n")
	s = strings.ReplaceAll(s, "<w:br/>", "\n")
	s = strings.ReplaceAll(s, "<w:tab/>", "\t")
	s = reXMLTag.ReplaceAllString(s, "")
	s = strings.ReplaceAll(s, "&lt;", "<")
	s = strings.ReplaceAll(s, "&gt;", ">")
	s = strings.ReplaceAll(s, "&quot;", `"`)
	s = strings.ReplaceAll(s, "&apos;", "'")
	s = strings.ReplaceAll(s, "&amp;", "&")
	return normalize(s), nil
}

func fromPDF(data []byte) (string, error) {
	r, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", fmt.Errorf("не удалось открыть PDF: %w", err)
	}
	rd, err := r.GetPlainText()
	if err != nil {
		return "", fmt.Errorf("не удалось извлечь текст из PDF: %w — конвертируйте в DOCX или TXT", err)
	}
	b, err := io.ReadAll(rd)
	if err != nil {
		return "", fmt.Errorf("чтение текста PDF: %w", err)
	}
	out := normalize(string(b))
	if strings.TrimSpace(out) == "" {
		return "", fmt.Errorf("из PDF не удалось извлечь текст (возможно, скан без текстового слоя) — конвертируйте в DOCX или TXT")
	}
	return out, nil
}

func normalize(s string) string {
	s = reXMLSpace.ReplaceAllString(s, "\n\n")
	if !utf8.ValidString(s) {
		s = strings.ToValidUTF8(s, "")
	}
	return strings.TrimSpace(s)
}

// Chunk is one citable piece of an uploaded regulation.
type Chunk struct {
	Seq    int
	Clause string
	Title  string
	Text   string
}

var reLeadingClause = regexp.MustCompile(`^(\d{1,2}(?:\.\d{1,2}){0,2})[.)]?\s`)

// ChunkText splits a regulation into citation-sized pieces (~≤900 chars),
// preferring paragraph and sentence boundaries.
func ChunkText(text string) []Chunk {
	paragraphs := strings.Split(text, "\n")
	var out []Chunk
	var buf strings.Builder
	seq := 0

	flush := func(force bool) {
		if buf.Len() == 0 {
			return
		}
		body := strings.TrimSpace(buf.String())
		buf.Reset()
		if len([]rune(body)) < 40 && !force {
			return
		}
		clause := fmt.Sprintf("Фрагмент %d", seq+1)
		if m := reLeadingClause.FindStringSubmatch(body); m != nil {
			clause = "п. " + strings.TrimSuffix(m[1], ".")
		}
		title := ""
		if r := []rune(body); len(r) > 70 {
			title = strings.TrimSpace(string(r[:70])) + "…"
		} else {
			title = body
		}
		seq++
		out = append(out, Chunk{Seq: seq, Clause: clause, Title: title, Text: body})
	}

	for _, para := range paragraphs {
		para = strings.TrimSpace(para)
		if para == "" {
			if buf.Len() > 500 {
				flush(false)
			}
			continue
		}
		if buf.Len() > 0 {
			buf.WriteString("\n")
		}
		buf.WriteString(para)
		// Keep chunks citation-sized: cut at a sentence end near the limit.
		if len([]rune(buf.String())) >= 850 {
			flush(false)
		}
	}
	flush(false)
	return out
}
