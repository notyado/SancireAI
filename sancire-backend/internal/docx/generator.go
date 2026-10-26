package docx

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	reFootnoteBrackets = regexp.MustCompile(`\[\d+\]`)
	reUnderlines       = regexp.MustCompile(`_{3,}`)
	reEmptyQuotes      = regexp.MustCompile(`«\s*»`)
	reChoiceBrackets   = regexp.MustCompile(`\[([^[\]]*/[^[\]]*)\]`)
	reMultiSpace       = regexp.MustCompile(` {2,}`)
	reSpaceBeforePunct = regexp.MustCompile(` +([.,;:)])`)
	reLeftoverToken    = regexp.MustCompile(`\{\{[^{}]*\}\}`)
	reWhenBlock        = regexp.MustCompile(`\{\{when +([A-Za-z0-9_]+) *= *([^}]*)\}\}([\s\S]*?)\{\{/when\}\}`)
	reParaOpen         = regexp.MustCompile(`<w:p(?:>| [^>]*>)`)
	reParaClose        = regexp.MustCompile(`</w:p>`)
	reCellOpen         = regexp.MustCompile(`<w:tc>`)
	reCellClose        = regexp.MustCompile(`</w:tc>`)
	reTextRun          = regexp.MustCompile(`<w:t(?: [^>]*)?>(.*?)</w:t>`)
	reAnyTag           = regexp.MustCompile(`<[^>]+>`)
)

// GenerateModuleDocx creates a clean, filled .docx based on a module template.
// The template file itself is never modified (п. 3.1 Методических рекомендаций).
func GenerateModuleDocx(templatesDir, moduleID string, values map[string]string) ([]byte, string, error) {
	mod, ok := ModulesRegistry[moduleID]
	if !ok {
		return nil, "", fmt.Errorf("module %s not found in registry", moduleID)
	}
	// Required values and template tokens are validated below, before any
	// substitution happens (the template file itself is only ever read — AC-07).
	tplPath := filepath.Join(templatesDir, mod.FileName)
	tplData, err := os.ReadFile(tplPath)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read template %s: %w", tplPath, err)
	}
	// One pass over the module fields: required values must be present, and
	// every non-routing field must exist as a token in the template — otherwise
	// the generated document would silently miss a requisite.
	tokens := TemplateTokens(moduleID)
	for _, field := range mod.Fields {
		if field.Required && !field.Routing && strings.TrimSpace(values[field.Key]) == "" {
			return nil, "", fmt.Errorf("поле «%s» является обязательным", field.Label)
		}
		if !field.Routing && !tokens[field.Key] {
			return nil, "", fmt.Errorf("шаблон модуля %s не содержит поля %q", moduleID, field.Key)
		}
	}

	zipReader, err := zip.NewReader(bytes.NewReader(tplData), int64(len(tplData)))
	if err != nil {
		return nil, "", fmt.Errorf("invalid docx zip: %w", err)
	}
	outBuf := new(bytes.Buffer)
	zipWriter := zip.NewWriter(outBuf)
	for _, file := range zipReader.File {
		rc, err := file.Open()
		if err != nil {
			return nil, "", err
		}
		content, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return nil, "", err
		}
		if file.Name == "word/document.xml" {
			cleanedXML, err := processDocumentXML(string(content), values)
			if err != nil {
				return nil, "", err
			}
			header, err := zip.FileInfoHeader(file.FileInfo())
			if err != nil {
				return nil, "", err
			}
			header.Name = file.Name
			header.Method = zip.Deflate
			w, err := zipWriter.CreateHeader(header)
			if err != nil {
				return nil, "", err
			}
			if _, err := w.Write([]byte(cleanedXML)); err != nil {
				return nil, "", err
			}
		} else {
			w, err := zipWriter.Create(file.Name)
			if err != nil {
				return nil, "", err
			}
			if _, err := w.Write(content); err != nil {
				return nil, "", err
			}
		}
	}
	if err := zipWriter.Close(); err != nil {
		return nil, "", fmt.Errorf("failed to finalize docx zip: %w", err)
	}
	outFileName := fmt.Sprintf("%s_%s.docx", mod.Code, sanitizeFileName(mod.Title))
	return outBuf.Bytes(), outFileName, nil
}

// reUnsafeFileName covers the characters that are illegal in file names on
// Windows/macOS/Linux or would break a Content-Disposition header.
var reUnsafeFileName = regexp.MustCompile(`[/\\\"'<>|:*?\x00-\x1f]+`)

func sanitizeFileName(s string) string {
	return strings.TrimSpace(reUnsafeFileName.ReplaceAllString(s, "-"))
}

type paraSpan struct {
	openStart int
	openEnd   int // index just after the opening tag
	closeEnd  int // index just after </w:p>
	inCell    bool
}

// collectParagraphs locates every top-level <w:p>…</w:p> interval and whether
// it lives inside a table cell (cells must keep at least one paragraph).
func collectParagraphs(xml string) []paraSpan {
	var spans []paraSpan
	cellDepth := 0
	cursor := 0
	for {
		loc := reParaOpen.FindStringIndex(xml[cursor:])
		if loc == nil {
			break
		}
		openStart := cursor + loc[0]
		openEnd := cursor + loc[1]
		closeLoc := reParaClose.FindStringIndex(xml[openEnd:])
		if closeLoc == nil {
			break
		}
		closeEnd := openEnd + closeLoc[1]
		// Track table cell depth over the gap before this paragraph.
		gap := xml[cursor:openStart]
		cellDepth += len(reCellOpen.FindAllString(gap, -1)) - len(reCellClose.FindAllString(gap, -1))
		spans = append(spans, paraSpan{
			openStart: openStart,
			openEnd:   openEnd,
			closeEnd:  closeEnd,
			inCell:    cellDepth > 0,
		})
		cursor = closeEnd
	}
	// Account for cells opened after the last paragraph (not possible in valid
	// files, but keep the depth computation honest).
	return spans
}

// transformText applies value substitution and the final cleanup required by
// п. 3.3 Методических рекомендаций to a single text run.
func transformText(raw string, values map[string]string) (string, error) {
	out := raw
	// Conditional fragments: {{when key=prefix}}text{{/when}} — kept only when
	// the answered value matches the prefix (used for the special Matrix
	// conditions: transfer of IS access, IT-project wording).
	var whenErr error
	out = reWhenBlock.ReplaceAllStringFunc(out, func(m string) string {
		sub := reWhenBlock.FindStringSubmatch(m)
		if sub == nil {
			return m
		}
		key, want, inner := sub[1], sub[2], sub[3]
		val, ok := values[key]
		if !ok {
			whenErr = fmt.Errorf("неизвестный параметр шаблона {{%s}}", key)
			return m
		}
		if strings.Contains(normalizeAnswer(val), normalizeAnswer(want)) {
			return inner
		}
		return ""
	})
	if whenErr != nil {
		return "", whenErr
	}
	// {{key}} token substitution (values are XML-escaped because they are
	// spliced into the XML text node directly).
	for {
		i := strings.Index(out, "{{")
		if i < 0 {
			break
		}
		j := strings.Index(out[i:], "}}")
		if j < 0 {
			break
		}
		key := out[i+2 : i+j]
		val, ok := values[key]
		if !ok {
			return "", fmt.Errorf("неизвестный параметр шаблона {{%s}}", key)
		}
		out = out[:i] + escapeXMLText(val) + out[i+j+2:]
	}
	// Footnote markers [1], [2], ...
	out = reFootnoteBrackets.ReplaceAllString(out, "")
	// Unused alternatives in brackets: [проекту / проектам] -> проекту
	out = reChoiceBrackets.ReplaceAllStringFunc(out, func(m string) string {
		inner := m[1 : len(m)-1]
		first := strings.TrimSpace(strings.Split(inner, "/")[0])
		return first
	})
	// Fill lines
	out = reUnderlines.ReplaceAllString(out, "")
	// Leftovers of removed date/number templates: «»
	out = reEmptyQuotes.ReplaceAllString(out, "")
	out = reMultiSpace.ReplaceAllString(out, " ")
	// «… с 01.11.2026 .» → «… с 01.11.2026.»
	out = reSpaceBeforePunct.ReplaceAllString(out, "$1")
	out = strings.TrimRight(out, " ")
	return out, nil
}

func escapeXMLText(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	s = strings.ReplaceAll(s, "'", "&apos;")
	return s
}

// normalizeAnswer lowercases, folds ё→е and trims — used to match answered
// values against {{when ...}} conditions.
func normalizeAnswer(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, "ё", "е")
	return s
}

func unescapeXMLText(s string) string {
	r := strings.NewReplacer(
		"&lt;", "<",
		"&gt;", ">",
		"&quot;", "\"",
		"&apos;", "'",
		"&nbsp;", " ",
		"&amp;", "&", // must be last
	)
	return r.Replace(s)
}

// processDocumentXML fills {{...}} tokens, removes unused alternatives and
// strips the service blocks (approval grif, module stamp, closing notes) while
// preserving the registration and electronic-signature zones.
func processDocumentXML(docXML string, values map[string]string) (string, error) {
	spans := collectParagraphs(docXML)
	if len(spans) == 0 {
		return "", fmt.Errorf("в шаблоне не найдено ни одного абзаца")
	}
	var (
		out      strings.Builder
		lastCopied int
		inGrif   bool
		inNotes  bool
	)
	for _, span := range spans {
		paraXML := docXML[span.openStart:span.closeEnd]
		// Plain text of the paragraph (before transform).
		origText := paragraphText(paraXML)
		drop := false
		keepOverride := strings.Contains(origText, "НЕ УДАЛЯТЬ")

		// The approval grif runs from «УТВЕРЖДАЮ» up to and including the
		// service lines («МОДУЛЬНЫЙ ДОКУМЕНТ», crest); it ends at the
		// organisation name / act kind line, which must be preserved.
		isGrifBoundary := strings.Contains(origText, "НАИМЕНОВАНИЕ ОРГАНИЗАЦИИ") ||
			strings.Contains(origText, "ПРИКАЗ") || strings.Contains(origText, "РАСПОРЯЖЕНИЕ")
		if strings.Contains(origText, "УТВЕРЖДАЮ") {
			inGrif = true
		}
		if !keepOverride {
			if inGrif && !isGrifBoundary {
				drop = true
			}
			if inNotes {
				drop = true
			}
			if strings.Contains(origText, "Примечания") {
				inNotes = true
				drop = true
			}
			if strings.Contains(origText, "МОДУЛЬНЫЙ ДОКУМЕНТ") ||
				strings.Contains(origText, "место для") || strings.Contains(origText, "изображе") {
				drop = true
			}
		}
		if isGrifBoundary {
			inGrif = false
		}
		if !keepOverride {
			// Apply token substitution + cleanup inside each text run.
			var runErr error
			updated := reTextRun.ReplaceAllStringFunc(paraXML, func(run string) string {
				if runErr != nil {
					return run
				}
				m := reTextRun.FindStringSubmatch(run)
				if m == nil {
					return run
				}
				newText, err := transformText(m[1], values)
				if err != nil {
					runErr = err
					return run
				}
				if strings.Contains(run, "xml:space=\"preserve\"") {
					return `<w:t xml:space="preserve">` + newText + `</w:t>`
				}
				return `<w:t>` + newText + `</w:t>`
			})
			if runErr != nil {
				return "", runErr
			}
			paraXML = updated
		}

		// Drop the whole paragraph when it became an empty fill line outside
		// a table cell (table cells must keep at least one paragraph, so an
		// emptied cell paragraph stays as a blank line).
		newText := stripMarkupForCheck(paraXML)
		origVisible := stripMarkupForCheck(origText)
		if !drop && newText == "" && origVisible != "" && !span.inCell {
			drop = true
		}
		if strings.Contains(paraXML, "{{") {
			return "", fmt.Errorf("не все параметры шаблона подставлены: %s", strings.Join(reLeftoverToken.FindAllString(paraXML, -1), ", "))
		}

		// Emit the gap since the previous paragraph, then the paragraph
		// itself unless it was dropped.
		out.WriteString(docXML[lastCopied:span.openStart])
		if !drop {
			out.WriteString(paraXML)
		}
		lastCopied = span.closeEnd
	}
	out.WriteString(docXML[lastCopied:])
	return out.String(), nil
}

// paragraphText concatenates the content of all w:t runs of a paragraph XML.
func paragraphText(paraXML string) string {
	var b strings.Builder
	for _, m := range reTextRun.FindAllStringSubmatch(paraXML, -1) {
		b.WriteString(unescapeXMLText(m[1]))
	}
	return b.String()
}

// stripMarkupForCheck removes XML tags from a paragraph fragment and trims it,
// leaving only the visible text.
func stripMarkupForCheck(s string) string {
	return strings.TrimSpace(reAnyTag.ReplaceAllString(s, ""))
}
