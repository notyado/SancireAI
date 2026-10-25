package docx
import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"regexp"
	"strings"
	"sancireai/backend/internal/matrix"
)
const (
	MaxDocxUploadSize      = 15 * 1024 * 1024 // 15 MB
	MaxDocxDecompressSize  = 60 * 1024 * 1024 // 60 MB
	MaxZipEntries          = 150
)
var (
	reBracketPlaceholder = regexp.MustCompile(`\[\d+\]`)
	reUnderlineSequence  = regexp.MustCompile(`_{3,}`)
	reEmptyDatePattern   = regexp.MustCompile(`«___»|\.\.\.20__|\b20___`)
)
// AuditDocx performs comprehensive validation of an uploaded DOCX document
func AuditDocx(data []byte, filename string, overrideModuleID string) (*AuditReport, error) {
	if int64(len(data)) > MaxDocxUploadSize {
		return nil, fmt.Errorf("размер файла (%d байт) превышает допустимый лимит 15 МБ (п. 2.1 Инструкции)", len(data))
	}
	zipReader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("файл не является корректным DOCX архивом: %w", err)
	}
	if len(zipReader.File) > MaxZipEntries {
		return nil, fmt.Errorf("превышено допустимое число элементов архива (защита от zip-бомб)")
	}
	var totalDecompressed int64
	var docXMLBytes []byte
	for _, f := range zipReader.File {
		totalDecompressed += int64(f.UncompressedSize64)
		if totalDecompressed > MaxDocxDecompressSize {
			return nil, fmt.Errorf("превышен лимит распакованного содержимого (защита от zip-бомб)")
		}
		if f.Name == "word/document.xml" {
			rc, err := f.Open()
			if err != nil {
				return nil, fmt.Errorf("ошибка чтения word/document.xml: %w", err)
			}
			docXMLBytes, err = io.ReadAll(io.LimitReader(rc, MaxDocxDecompressSize))
			rc.Close()
			if err != nil {
				return nil, fmt.Errorf("ошибка распаковки document.xml: %w", err)
			}
		}
	}
	if len(docXMLBytes) == 0 {
		return nil, fmt.Errorf("в архиве отсутствует основной документ word/document.xml")
	}
	// Extract plain text runs from document.xml
	textContent, err := extractPlainTextFromXML(docXMLBytes)
	if err != nil {
		return nil, fmt.Errorf("ошибка XML-парсинга документа: %w", err)
	}
	// Determine document kind and title
	title := extractTitle(textContent)
	detectedKind := matrix.DetectDocKind(title, textContent)
	// Determine matched module
	matchedModuleID := overrideModuleID
	if matchedModuleID == "" {
		matchedModuleID = matchModuleByText(title, textContent)
	}
	var findings []AuditFinding
	findingIdx := 1
	addFinding := func(sev AuditSeverity, title, desc, loc, rule, rec string) {
		findings = append(findings, AuditFinding{
			ID:             fmt.Sprintf("F-%03d", findingIdx),
			Severity:       sev,
			Title:          title,
			Description:    desc,
			Location:       loc,
			RuleCitation:   rule,
			Recommendation: rec,
		})
		findingIdx++
	}
	// 1. Check for unreplaced bracket placeholders [1], [2], ...
	bracketMatches := reBracketPlaceholder.FindAllString(textContent, -1)
	if len(bracketMatches) > 0 {
		addFinding(
			SeverityCritical,
			"Не заменены служебные сноски и плейсхолдеры",
			fmt.Sprintf("В тексте документа обнаружены служебные маркеры шаблона: %s. Согласно п. 3.3 Методических рекомендаций квадратные скобки и номера сносок обязаны быть удалены.", strings.Join(uniqueStrings(bracketMatches), ", ")),
			"Основной текст проекта",
			"Методические рекомендации по работе с модульными документами, п. 3.3",
			"Заполните соответствующие параметры и удалите квадратные скобки со сносками.",
		)
	}
	// 2. Check for unfilled underline placeholders _____
	underlineMatches := reUnderlineSequence.FindAllString(textContent, -1)
	if len(underlineMatches) > 0 {
		addFinding(
			SeverityCritical,
			"Обнаружены незаполненные поля и линии подчеркиваний",
			fmt.Sprintf("В документе найдено %d фрагментов с прочерками «_____». Согласно п. 3.3 Рекомендаций все прочерки и линии должны быть заменены реальными реквизитами.", len(underlineMatches)),
			"Поля реквизитов / преамбула",
			"Методические рекомендации, п. 3.3; Инструкция по ДОУ, Глава 8",
			"Замените прочерки на конкретные данные (даты, ФИО, подразделения) или удалите неиспользуемые строки.",
		)
	}
	// 3. Check for empty date placeholders
	dateMatches := reEmptyDatePattern.FindAllString(textContent, -1)
	if len(dateMatches) > 0 {
		addFinding(
			SeverityWarning,
			"Не указана точная календарная дата",
			fmt.Sprintf("Обнаружены незаполненные шаблоны дат (%s).", strings.Join(uniqueStrings(dateMatches), ", ")),
			"Даты вступления в силу / сроки исполнения",
			"Инструкция по ДОУ, п. 8.4",
			"Укажите точную дату согласования, вступления в силу или срок отчета.",
		)
	}
	// 4. Check for presence of EP visualization zone
	if !strings.Contains(textContent, "ВИЗУАЛИЗАЦИЯ ЭП") && !strings.Contains(textContent, "ЭЛЕКТРОННОЙ ПОДПИСИ") && !strings.Contains(textContent, "ЭП") {
		addFinding(
			SeverityWarning,
			"Отсутствует обязательная зона визуализации электронной подписи",
			"В документе не найдена служебная зона для отметки об электронной подписи (ЭП). Согласно п. 8.12 Инструкции зона ЭП является обязательным реквизитом электронного документа.",
			"Нижняя часть документа / зона подписи",
			"Инструкция по ДОУ, п. 8.12; Методические рекомендации, п. 3.3",
			"Восстановите служебный блок «ВИЗУАЛИЗАЦИЯ ЭП (НЕ УДАЛЯТЬ)» перед отправкой на подписание.",
		)
	}
	// 5. Check for registration number zone
	if !strings.Contains(textContent, "РЕГИСТРАЦИОННЫЙ НОМЕР") && !strings.Contains(textContent, "ДАТА /") && !strings.Contains(textContent, "№") {
		addFinding(
			SeverityWarning,
			"Отсутствует зона даты и регистрационного номера",
			"В шапке документа не найдено поле для даты и номера регистрации. При регистрации в СЭД реквизиты проставляются на первом листе (п. 5.4 Инструкции).",
			"Шапка документа (под наименованием вида акта)",
			"Инструкция по ДОУ, п. 5.4 и п. 8.3",
			"Добавьте служебную отметку для регистрации документа.",
		)
	}
	// 6. Semantic deviation from the reference module (FR-021 / п. 1.3
	// Методических рекомендаций): every module keeps a set of constant anchor
	// phrases; if a matched module lost one of them, the wording was changed.
	customWording := false
	if matchedModuleID != "" {
		if mod, ok := ModulesRegistry[matchedModuleID]; ok {
			var missing []string
			normText := normalizeForCompare(textContent)
			for _, anchor := range mod.Anchors {
				if !strings.Contains(normText, normalizeForCompare(anchor)) {
					missing = append(missing, anchor)
				}
			}
			if len(missing) > 0 {
				customWording = true
				addFinding(
					SeverityWarning,
					"Изменение постоянных формулировок эталонного модуля",
					fmt.Sprintf("Не найдены постоянные формулировки утвержденного модуля %s: %s. В соответствии с п. 1.3 Методических рекомендаций документ теряет привилегию сокращенного согласования и должен пройти полный круг визирования по Главе 10 Инструкции по ДОУ.",
						mod.Code, strings.Join(missing, "; ")),
					"Распорядительная часть",
					"Методические рекомендации по модульным документам, п. 1.3",
					"Верните постоянные формулировки эталонного модуля или согласуйте документ по полному маршруту (Глава 10).",
				)
			}
		}
	}
	// Determine condition flags
	hasISAccessChange := strings.Contains(strings.ToLower(textContent), "доступ") &&
		(strings.Contains(strings.ToLower(textContent), "информацион") || strings.Contains(strings.ToLower(textContent), "ис") || strings.Contains(strings.ToLower(textContent), "систем"))
	isITProject := strings.Contains(strings.ToLower(textContent), "ит-проект") ||
		strings.Contains(strings.ToLower(textContent), "информационн") ||
		strings.Contains(strings.ToLower(textContent), "программн") ||
		strings.Contains(strings.ToLower(textContent), "цифров")
	// Calculate visa routing using Matrix engine
	docCtx := matrix.DocumentContext{
		Kind:              detectedKind,
		Title:             title,
		HasISAccessChange: hasISAccessChange,
		IsITProject:       isITProject,
		MatchedModuleID:   matchedModuleID,
		CustomWording:     customWording,
	}
	visaRouting := matrix.CalculateVisas(docCtx)
	// Tally severities
	criticals := 0
	warnings := 0
	infos := 0
	for _, f := range findings {
		switch f.Severity {
		case SeverityCritical:
			criticals++
		case SeverityWarning:
			warnings++
		case SeverityInfo:
			infos++
		}
	}
	var status TrafficLight
	var summary string
	if criticals > 0 {
		status = TrafficLightRed
		summary = fmt.Sprintf("Обнаружено %d критических замечаний: документ не может быть направлен на согласование до устранения ошибок.", criticals)
	} else if warnings > 0 {
		status = TrafficLightYellow
		summary = fmt.Sprintf("Документ содержит %d предупреждений. Рекомендуется исправить замечания перед отправкой согласующим.", warnings)
	} else {
		status = TrafficLightGreen
		summary = "Документ полностью соответствует требованиям Инструкции по ДОУ и готов к визированию."
	}
	var moduleTitle string
	if m, ok := ModulesRegistry[matchedModuleID]; ok {
		moduleTitle = m.Title
	}
	return &AuditReport{
		FileName:        filename,
		FileSize:        int64(len(data)),
		Status:          status,
		StatusSummary:   summary,
		DetectedKind:    detectedKind,
		DetectedModule:  moduleTitle,
		MatchedModuleID: matchedModuleID,
		TotalFindings:   len(findings),
		CriticalCount:   criticals,
		WarningCount:    warnings,
		InfoCount:       infos,
		Findings:        findings,
		VisaRouting:     visaRouting,
	}, nil
}
func extractPlainTextFromXML(xmlData []byte) (string, error) {
	decoder := xml.NewDecoder(bytes.NewReader(xmlData))
	var sb strings.Builder
	for {
		tok, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Local == "p" {
				sb.WriteString("\n")
			}
		case xml.CharData:
			sb.Write(t)
		}
	}
	return sb.String(), nil
}
func extractTitle(text string) string {
	lines := strings.Split(text, "\n")
	for _, l := range lines {
		trimmed := strings.TrimSpace(l)
		if strings.HasPrefix(trimmed, "О ") || strings.HasPrefix(trimmed, "Об ") {
			return trimmed
		}
	}
	for _, l := range lines {
		trimmed := strings.TrimSpace(l)
		if len(trimmed) > 10 && len(trimmed) < 120 && !strings.Contains(trimmed, "УТВЕРЖДАЮ") && !strings.Contains(trimmed, "ОБЯЗЫВАЮ") && !strings.Contains(trimmed, "ПРИКАЗЫВАЮ") {
			return trimmed
		}
	}
	return "Проект документа"
}
func matchModuleByText(title, content string) string {
	full := strings.ToLower(title + " " + content)
	if strings.Contains(full, "вводе в эксплуатацию") && strings.Contains(full, "информацион") {
		return "M-01"
	}
	if strings.Contains(full, "ответственных за делопроизводство") {
		return "M-02"
	}
	if strings.Contains(full, "приеме и передаче дел") {
		return "M-03"
	}
	if strings.Contains(full, "утверждении и введении в действие") {
		return "M-04"
	}
	if strings.Contains(full, "внутреннего аудита") {
		return "M-05"
	}
	if strings.Contains(full, "проектного комитета") {
		return "M-06"
	}
	if strings.Contains(full, "рабочей группы") {
		return "M-07"
	}
	return ""
}
func uniqueStrings(slice []string) []string {
	seen := make(map[string]bool)
	var res []string
	for _, s := range slice {
		if !seen[s] {
			seen[s] = true
			res = append(res, s)
		}
	}
	return res
}

// normalizeForCompare folds case/whitespace/ё so anchor phrases match
// regardless of line breaks produced by the DOCX layout engine.
func normalizeForCompare(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, "ё", "е")
	var b strings.Builder
	prevSpace := false
	for _, r := range s {
		if r == '\n' || r == '\r' || r == '\t' || r == ' ' {
			if !prevSpace {
				b.WriteRune(' ')
				prevSpace = true
			}
			continue
		}
		prevSpace = false
		b.WriteRune(r)
	}
	return b.String()
}
