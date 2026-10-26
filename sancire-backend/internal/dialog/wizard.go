package dialog

import (
	"fmt"
	"regexp"
	"strings"

	"sancireai/backend/internal/docx"
)

// State persists per-conversation wizard progress inside Conversation.Draft.
type State struct {
	ModuleID string            `json:"moduleId"`
	Phase    string            `json:"phase"` // "ask" | "confirm"
	Step     int               `json:"step"`
	Values   map[string]string `json:"values"`
}

// WizardMeta is attached to every wizard message for the frontend UI.
type WizardMeta struct {
	ModuleID string            `json:"moduleId"`
	Code     string            `json:"code"`
	Title    string            `json:"title"`
	Phase    string            `json:"phase"`
	Step     int               `json:"step"` // 1-based while asking
	Total    int               `json:"total"`
	Field    *WizardField      `json:"field,omitempty"`
	Values   map[string]string `json:"values,omitempty"`
}

// WizardField describes the question currently on screen.
type WizardField struct {
	Key      string   `json:"key"`
	Label    string   `json:"label"`
	Question string   `json:"question"`
	Example  string   `json:"example,omitempty"`
	Note     string   `json:"note,omitempty"`
	Options  []string `json:"options,omitempty"`
	Required bool     `json:"required"`
}

var reDate = regexp.MustCompile(`^\d{2}\.\d{2}\.\d{4}$`)

func wizardFields(modID string) []docx.ModuleField {
	mod, ok := docx.ModulesRegistry[modID]
	if !ok {
		return nil
	}
	return mod.Fields
}

// finish builds an Outcome for a wizard turn: streams the answer, attaches
// wizard meta with clickable follow-ups and optionally persists the state.
func finish(req Request, answer string, intent string, st *State, fields []docx.ModuleField, persist bool) Outcome {
	meta := Meta{Intent: intent}
	if st != nil {
		meta.Wizard = wizardMeta(st, fields)
		meta.FollowUps = wizardFollowUps(st, fields)
	}
	emit(req.Token, answer)
	out := Outcome{Answer: answer, Meta: meta}
	if st != nil && persist {
		out.NewDraft = st
		out.ChangeDraft = true
	}
	return out
}

func (e *Engine) startWizard(req Request, modID string) Outcome {
	mod, ok := docx.ModulesRegistry[modID]
	if !ok {
		return e.reply(req, "Неизвестный модуль документа.", Meta{Intent: "wizard.error"})
	}
	fields := wizardFields(modID)
	if len(fields) == 0 {
		return e.reply(req, "Для модуля "+mod.Code+" не настроены поля.", Meta{Intent: "wizard.error"})
	}
	st := &State{ModuleID: modID, Phase: "ask", Step: 0, Values: map[string]string{}}
	answer := fmt.Sprintf(
		"Соберу параметры документа **«%s»** (%s).\nВопросов: %d — отвечайте в свободной форме, формат проверю сам.\n\n%s",
		mod.Title, mod.Code, len(fields), renderQuestion(st, fields, 0))
	return finish(req, answer, "wizard.start", st, fields, true)
}

func (e *Engine) startClarify(req Request) Outcome {
	var b strings.Builder
	b.WriteString("Уточните, какой документ нужен — по каждому модулю свой сценарий вопросов:\n")
	var follow []string
	for _, id := range docx.ModuleIDs() {
		mod := docx.ModulesRegistry[id]
		b.WriteString(fmt.Sprintf("\n- **%s** (%s)", mod.Title, mod.Code))
		follow = append(follow, fmt.Sprintf("%s (%s)", mod.Title, mod.Code))
	}
	b.WriteString("\n\nВыберите подходящий — и я задам вопросы по одному.")
	return e.reply(req, b.String(), Meta{
		Intent:     "wizard.clarify",
		Clarifying: true,
		FollowUps:  follow,
	})
}

// wizardTurn handles a message while a draft is active. ok=false means the
// message is a new command and should be processed by the general intents.
func (e *Engine) wizardTurn(req Request, st *State, text, norm string) (Outcome, bool) {
	fields := wizardFields(st.ModuleID)
	if len(fields) == 0 {
		return Outcome{}, false
	}

	// Cancel.
	if reCancel.MatchString(norm) {
		return Outcome{
			Answer:      "Диалог отменён — значения не сохранены. Могу начать заново в любой момент.",
			Meta:        Meta{Intent: "wizard.cancel", FollowUps: []string{"Сформировать документ по модулю", "Что ты умеешь?"}},
			ClearDraft:  true,
			ChangeDraft: true,
		}, true
	}

	// Restart with another module ("… (М-03)" chips or an explicit code).
	if m := reModuleCode.FindStringSubmatch(text); m != nil && ("M-0"+m[1]) != st.ModuleID {
		return e.startWizard(req, "M-0"+m[1]), true
	}

	// Summary on demand.
	if reSummaryAsk.MatchString(norm) {
		mod := docx.ModulesRegistry[st.ModuleID]
		ans := summaryText(st, fields, mod) + "\n\n" + renderQuestion(st, fields, st.Step)
		return finish(req, ans, "wizard.summary", st, fields, true), true
	}

	if st.Phase == "confirm" {
		return e.confirmTurn(req, st, fields, text, norm), true
	}
	if st.Step >= len(fields) {
		st.Phase = "confirm"
		return e.confirmTurn(req, st, fields, text, norm), true
	}

	field := fields[st.Step]

	// Skip.
	if reSkip.MatchString(norm) {
		st.Values[field.Key] = ""
		return e.advance(req, st, fields, field, true), true
	}

	// Validate & store the answer.
	value := strings.TrimSpace(text)
	if value == "" {
		return finish(req, "Ответ пуст — введите значение или скажите «Пропустить».\n\n"+renderQuestion(st, fields, st.Step), "wizard.retry", st, fields, true), true
	}
	// Choice fields must resolve to one of the registry options; other fields
	// that declare options are normalised when they match and kept verbatim
	// otherwise.
	if len(field.Options) > 0 {
		if picked, ok := matchOption(value, field.Options); ok {
			value = picked
		} else if field.Type == "choice" {
			ans := fmt.Sprintf("Нужен один из вариантов: %s.\n\n", strings.Join(field.Options, " / ")) + renderQuestion(st, fields, st.Step)
			return finish(req, ans, "wizard.retry", st, fields, true), true
		}
	}
	// Date fields must be ДД.ММ.ГГГГ — the type is declared in the registry,
	// so no per-key exceptions are needed.
	if field.Type == "date" && value != "" && !reDate.MatchString(value) {
		return finish(req, "Дата должна быть в формате ДД.ММ.ГГГГ — например 01.11.2026.\n\n"+renderQuestion(st, fields, st.Step), "wizard.retry", st, fields, true), true
	}
	st.Values[field.Key] = value
	for _, t := range field.SyncTargets {
		if t != field.Key {
			st.Values[t] = value
		}
	}
	return e.advance(req, st, fields, field, false), true
}

// matchOption resolves a free-form answer to one of the declared options: an
// exact match or an option mentioned inside a longer answer
// («возьму вариант разработанного» → «разработанного»).
func matchOption(value string, options []string) (string, bool) {
	v := normalizeText(value)
	for _, o := range options {
		ol := normalizeText(o)
		if v == ol || strings.Contains(v, ol) {
			return o, true
		}
	}
	return "", false
}

func (e *Engine) advance(req Request, st *State, fields []docx.ModuleField, field docx.ModuleField, skipped bool) Outcome {
	st.Step++
	if st.Step >= len(fields) {
		st.Phase = "confirm"
		mod := docx.ModulesRegistry[st.ModuleID]
		ans := "Принято (" + field.Label + ").\n\n" + summaryText(st, fields, mod) +
			"\n\nЕсли всё верно — **«Сформировать документ»**. Если нужно поправить — **«Изменить»**."
		return finish(req, ans, "wizard.confirm", st, fields, true)
	}
	ack := "Принято."
	if skipped {
		ack = "Поле пропущено. Если оно обязательное, вернёмся к нему перед формированием."
	}
	return finish(req, ack+"\n\n"+renderQuestion(st, fields, st.Step), "wizard.next", st, fields, true)
}

func (e *Engine) confirmTurn(req Request, st *State, fields []docx.ModuleField, text, norm string) Outcome {
	mod := docx.ModulesRegistry[st.ModuleID]
	if reEdit.MatchString(norm) {
		st.Phase = "ask"
		st.Step = 0
		ans := "Пройдём по значениям заново — уже введённые ответы подставлены. Скажите новый ответ или «Пропустить».\n\n" + renderQuestion(st, fields, 0)
		return finish(req, ans, "wizard.edit", st, fields, true)
	}
	if reFinalize.MatchString(norm) {
		return e.finalize(req, st, fields, mod)
	}
	ans := summaryText(st, fields, mod) + "\n\nСкажите **«Сформировать документ»** или **«Изменить»**."
	return finish(req, ans, "wizard.confirm", st, fields, true)
}

// finalize generates the docx; on validation errors it re-asks the missing field.
func (e *Engine) finalize(req Request, st *State, fields []docx.ModuleField, mod docx.ModuleDefinition) Outcome {
	for i, f := range fields {
		if f.Required && strings.TrimSpace(st.Values[f.Key]) == "" {
			st.Phase = "ask"
			st.Step = i
			ans := fmt.Sprintf("Перед формированием заполним обязательное поле: **%s**.\n\n", f.Label) + renderQuestion(st, fields, i)
			return finish(req, ans, "wizard.missing", st, fields, true)
		}
	}
	// GenerateModuleDocx only reads the collected values.
	data, filename, err := docx.GenerateModuleDocx(e.templatesDir, st.ModuleID, st.Values)
	if err != nil {
		return finish(req, "Не удалось сформировать документ: "+err.Error()+"\n\nСкажите «Изменить» — поправим значения и повторим.", "wizard.error", st, fields, true)
	}
	preview := buildPreview(st, fields)
	answer := fmt.Sprintf(
		"Документ сформирован: **%s**\n\nФайл (.docx) прикреплён к сообщению ниже — скачайте его кнопкой «Скачать».\n\n"+
			"Что сделано: служебные сноски и подчёркивания удалены, регистрационная зона и зона визуализации ЭП сохранены.\n"+
			"При желании проверьте готовый файл — просто загрузите его в чат.",
		filename)
	emit(req.Token, answer)
	return Outcome{
		Answer: answer,
		Meta: Meta{
			Intent: "wizard.done",
			Document: &DocumentCard{
				Title:    mod.Title,
				Kind:     string(mod.Kind),
				FileName: filename,
				Preview:  preview,
				ModuleID: st.ModuleID,
			},
			FollowUps: []string{"Проверить готовый документ", "Какой будет маршрут визирования?", "Сформировать ещё один документ"},
		},
		ClearDraft:  true,
		ChangeDraft: true,
		Artifact: &ArtifactOut{
			Data:     data,
			FileName: filename,
			Title:    mod.Title,
			Kind:     string(mod.Kind),
			Preview:  preview,
		},
	}
}

func buildPreview(st *State, fields []docx.ModuleField) string {
	var b strings.Builder
	for _, f := range fields {
		v := strings.TrimSpace(st.Values[f.Key])
		if v == "" {
			v = "—"
		}
		b.WriteString(fmt.Sprintf("%s: %s\n", f.Label, v))
	}
	return b.String()
}

func summaryText(st *State, fields []docx.ModuleField, mod docx.ModuleDefinition) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("**Собранные значения — %s (%s):**\n", mod.Title, mod.Code))
	for _, f := range fields {
		v := strings.TrimSpace(st.Values[f.Key])
		if v == "" {
			v = "_пропущено_"
		}
		b.WriteString(fmt.Sprintf("- %s: %s\n", f.Label, v))
	}
	return b.String()
}

func renderQuestion(st *State, fields []docx.ModuleField, idx int) string {
	if idx < 0 || idx >= len(fields) {
		return ""
	}
	f := fields[idx]
	q := f.Question
	if q == "" {
		q = f.Label
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("**Вопрос %d/%d — %s**\n", idx+1, len(fields), f.Label))
	b.WriteString(q)
	b.WriteString("\n")
	if f.HelperNote != "" {
		b.WriteString(fmt.Sprintf("_%s_\n", f.HelperNote))
	}
	if f.Placeholder != "" {
		b.WriteString(fmt.Sprintf("_Пример: %s_\n", f.Placeholder))
	}
	if len(f.Options) > 0 {
		b.WriteString(fmt.Sprintf("Варианты: %s\n", strings.Join(f.Options, " / ")))
	}
	return b.String()
}

func wizardMeta(st *State, fields []docx.ModuleField) *WizardMeta {
	mod := docx.ModulesRegistry[st.ModuleID]
	wm := &WizardMeta{
		ModuleID: st.ModuleID,
		Code:     mod.Code,
		Title:    mod.Title,
		Phase:    st.Phase,
		Total:    len(fields),
		Values:   st.Values,
	}
	if st.Phase == "ask" && st.Step < len(fields) {
		f := fields[st.Step]
		wm.Step = st.Step + 1
		wm.Field = &WizardField{
			Key:      f.Key,
			Label:    f.Label,
			Question: f.Question,
			Example:  f.Placeholder,
			Note:     f.HelperNote,
			Options:  f.Options,
			Required: f.Required,
		}
	} else {
		wm.Step = len(fields)
	}
	return wm
}

// wizardFollowUps returns the clickable buttons for the current question.
func wizardFollowUps(st *State, fields []docx.ModuleField) []string {
	if st.Phase == "confirm" {
		return []string{"Сформировать документ", "Изменить", "Отмена"}
	}
	var out []string
	if st.Step < len(fields) {
		if f := fields[st.Step]; len(f.Options) > 0 {
			out = append(out, f.Options...)
		}
	}
	out = append(out, "Пропустить", "Показать, что уже собрано", "Отмена")
	return out
}
