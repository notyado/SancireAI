package dialog

import (
	"encoding/base64"
	"fmt"
	"strings"
	"testing"

	"sancireai/backend/internal/docx"
)

// newTestEngine points the dialog at the repo corpus and templates.
func newTestEngine(t *testing.T) *Engine {
	t.Helper()
	return NewEngine("../../templates", "../../corpus")
}

func startDraft(out Outcome) any { return out.NewDraft }

func TestWizardFullCycleM01(t *testing.T) {
	e := newTestEngine(t)

	// 1. Trigger by natural phrase.
	out := e.Respond(Request{Text: "Приказ о вводе информационной системы в эксплуатацию"})
	if out.Meta.Intent != "wizard.start" {
		t.Fatalf("expected wizard.start, got %q (%s)", out.Meta.Intent, out.Answer)
	}
	if out.Meta.Wizard == nil || out.Meta.Wizard.ModuleID != "M-01" {
		t.Fatalf("expected M-01 wizard, got %+v", out.Meta.Wizard)
	}
	if !strings.Contains(out.Answer, "Вопрос 1/") {
		t.Fatalf("first question missing: %s", out.Answer)
	}
	st := startDraft(out)
	fields := wizardFields("M-01")
	answers := map[string]string{
		"is_name":          "Пилотная ИС",
		"basis_doc":        "договор № 42 от 01.10.2026",
		"modification_type": "разработанного",
		"tz_requisites":    "ТЗ-7 от 15.10.2026",
		"start_date":       "01.11.2026",
		"customer_dept":    "Департамент технологических сервисов",
		"purpose":          "обеспечение ведения реестра",
		"controller_person":"Петров П.П.",
		"signatory":        "Заместитель Председателя Организации Иванов И.И.",
	}
	var last Outcome
	for i, f := range fields {
		ans := answers[f.Key]
		if ans == "" {
			ans = "10.11.2026"
		}
		last = e.Respond(Request{Text: ans, Draft: st})
		if last.Meta.Intent == "wizard.retry" {
			t.Fatalf("retry at field %d (%s): %s", i, f.Key, last.Answer)
		}
		if last.Meta.Wizard == nil {
			t.Fatalf("no wizard meta at field %d", i)
		}
		st = last.NewDraft
		if st == nil {
			// keep previous state if not persisted
			t.Fatalf("draft lost at field %d", i)
		}
	}
	if st.(*State).Phase != "confirm" {
		t.Fatalf("expected confirm phase, got %s", st.(*State).Phase)
	}
	// 2. Confirm & generate.
	out = e.Respond(Request{Text: "Сформировать документ", Draft: st})
	if out.Artifact == nil {
		t.Fatalf("no artifact: %s", out.Answer)
	}
	if out.Meta.Document == nil || out.Meta.Document.FileName == "" {
		t.Fatalf("no document card: %+v", out.Meta.Document)
	}
	if !strings.HasSuffix(out.Meta.Document.FileName, ".docx") {
		t.Fatalf("bad filename %q", out.Meta.Document.FileName)
	}
	if out.ClearDraft != true {
		t.Fatalf("draft must be cleared after generation")
	}
	// 3. Audit the generated file from chat (base64 round trip).
	b64 := base64.StdEncoding.EncodeToString(out.Artifact.Data)
	auditOut := e.Respond(Request{Text: "Проверь файл", Files: []File{{Name: out.Meta.Document.FileName, Data: b64}}})
	if auditOut.Meta.Audit == nil {
		t.Fatalf("no audit report: %s", auditOut.Answer)
	}
	if auditOut.Meta.Audit.CriticalCount != 0 {
		t.Fatalf("generated doc must audit clean, got %d criticals: %s", auditOut.Meta.Audit.CriticalCount, auditOut.Answer)
	}
	if auditOut.Meta.Audit.VisaRouting.AppliedRules == nil || len(auditOut.Meta.Audit.VisaRouting.AppliedRules) == 0 {
		t.Fatalf("matrix justification missing")
	}
}

func TestScenarioQuestions(t *testing.T) {
	e := newTestEngine(t)
	cases := []struct {
		name      string
		query     string
		wantPart  string
		wantSrc   string // substring expected in at least one source ID
		wantIntent string
	}{
		{"визирование ИС", "кто визирует приказ о вводе информационной системы", "", "matrix", "answer"},
		{"служебка без письма", "можно ли направить внутреннюю служебную записку без сопроводительного письма", "", "od184", "answer"},
		{"реестр замечаний", "как оформить реестр замечаний к проекту документа", "", "recom", "answer"},
		{"согласование большого документа", "как организовать согласование большого нормативного документа", "Порядок", "", "answer.process"},
		{"сроки", "сроки согласования проекта документа", "", "", "answer"},
		{"письмо к записке", "сопроводительное письмо к служебной записке — структура и порядок направления", "", "", "answer"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := e.Respond(Request{Text: tc.query})
			got := out.Meta.Intent
			okIntent := got == tc.wantIntent || (tc.wantIntent == "answer" && got == "answer.process")
			if !okIntent {
				t.Errorf("intent = %q want %q (answer: %s)", out.Meta.Intent, tc.wantIntent, out.Answer)
			}
			if out.Answer == "" {
				t.Fatalf("empty answer")
			}
			if len(out.Meta.Sources) == 0 {
				t.Errorf("no sources: %s", out.Answer)
			} else if tc.wantSrc != "" {
				found := false
				for _, s := range out.Meta.Sources {
					if strings.Contains(s.ID, tc.wantSrc) {
						found = true
					}
				}
				if !found {
					var ids []string
					for _, s := range out.Meta.Sources {
						ids = append(ids, s.ID)
					}
					t.Errorf("no source containing %q; got %v", tc.wantSrc, ids)
				}
			}
			if tc.wantPart != "" && !strings.Contains(out.Answer, tc.wantPart) {
				t.Errorf("answer misses %q: %s", tc.wantPart, out.Answer)
			}
		})
	}
}

func TestRefusalForOutOfScope(t *testing.T) {
	e := newTestEngine(t)
	out := e.Respond(Request{Text: "штраф за парковку во дворе"})
	if !out.Meta.NotFound {
		t.Fatalf("expected NotFound, got intent %q: %s", out.Meta.Intent, out.Answer)
	}
}

func TestSmallTalkAndFeedback(t *testing.T) {
	e := newTestEngine(t)
	if out := e.Respond(Request{Text: "привет"}); out.Meta.Intent != "greeting" {
		t.Errorf("greeting intent = %q", out.Meta.Intent)
	}
	if out := e.Respond(Request{Text: "что умеешь?"}); out.Meta.Intent != "help" {
		t.Errorf("help intent = %q", out.Meta.Intent)
	}
	if out := e.Respond(Request{Text: "предложение: сделай темную тему получше"}); out.Meta.Intent != "feedback" {
		t.Errorf("feedback intent = %q", out.Meta.Intent)
	}
	// Hard question must NOT start the wizard even with a module trigger.
	out := e.Respond(Request{Text: "кто визирует приказ о вводе информационной системы?"})
	if out.Meta.Intent == "wizard.start" {
		t.Errorf("hard question wrongly started a wizard")
	}
}

func TestWizardCancelAndSummary(t *testing.T) {
	e := newTestEngine(t)
	out := e.Respond(Request{Text: "Создай приказ о приеме и передаче дел"})
	if out.Meta.Intent != "wizard.start" || out.Meta.Wizard.ModuleID != "M-03" {
		t.Fatalf("expected M-03 wizard, got %q %+v", out.Meta.Intent, out.Meta.Wizard)
	}
	st := out.NewDraft
	out = e.Respond(Request{Text: "смеря", Draft: st}) // not a cancel
	out = e.Respond(Request{Text: "Отмена", Draft: st})
	if !out.ClearDraft {
		t.Fatalf("cancel must clear draft: %s", out.Answer)
	}
}

func TestAuditInviteIntent(t *testing.T) {
	e := newTestEngine(t)
	out := e.Respond(Request{Text: "Проверка готового модульного документа — что дальше?"})
	if out.Meta.Intent != "audit.invite" {
		t.Fatalf("intent = %q (%s)", out.Meta.Intent, out.Answer)
	}
	if !strings.Contains(out.Answer, "docx") {
		t.Fatalf("invite must mention .docx upload: %s", out.Answer)
	}
}

func TestMatrixVisaJustificationText(t *testing.T) {
	// The justifications must be deterministic strings naming matrix rows.
	data, filename, err := docx.GenerateModuleDocx("../../templates", "M-01", map[string]string{
		"is_name": "ИС Тест", "basis_doc": "док. 1", "modification_type": "разработанного",
		"tz_requisites": "ТЗ-1", "start_date": "01.11.2026", "customer_dept": "ДЭБ",
		"purpose": "цель", "controller_person": "Петров П.П.", "signatory": "Иванов И.И.",
	})
	if err != nil {
		t.Fatal(err)
	}
	rep, err := docx.AuditDocx(data, filename, "")
	if err != nil {
		t.Fatal(err)
	}
	foundRow := false
	for _, rule := range rep.VisaRouting.AppliedRules {
		if strings.Contains(rule, "Строка 3") && strings.Contains(rule, "информационных технологий") {
			foundRow = true
		}
		if !strings.Contains(rule, "Матрицы") {
			t.Errorf("rule lacks normative reference: %s", rule)
		}
	}
	if !foundRow {
		t.Fatalf("no row-3 justification: %v", rep.VisaRouting.AppliedRules)
	}
	fmt.Println("OK justification rules:", len(rep.VisaRouting.AppliedRules))
}
