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

func TestScenarioQuestions(t *testing.T) {
	e := newTestEngine(t)
	cases := []struct {
		name       string
		query      string
		wantPart   string
		wantSrc    string // substring expected in at least one source ID
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

func TestDocxAuditFromChat(t *testing.T) {
	e := newTestEngine(t)
	data, filename, err := docx.GenerateModuleDocx("../../templates", "M-01", map[string]string{
		"is_name":           "Пилотная ИС",
		"basis_doc":         "договор № 42 от 01.10.2026",
		"modification_type": "разработанного",
		"tz_requisites":     "ТЗ-7 от 15.10.2026",
		"start_date":        "01.11.2026",
		"customer_dept":     "Департамент технологических сервисов",
		"purpose":           "обеспечение ведения реестра",
		"controller_person": "Петров П.П.",
		"signatory":         "Заместитель Председателя Организации Иванов И.И.",
	})
	if err != nil {
		t.Fatal(err)
	}
	b64 := base64.StdEncoding.EncodeToString(data)
	auditOut := e.Respond(Request{Text: "Проверь файл", Files: []File{{Name: filename, Data: b64}}})
	if auditOut.Meta.Audit == nil {
		t.Fatalf("no audit report: %s", auditOut.Answer)
	}
	if auditOut.Meta.Audit.CriticalCount != 0 {
		t.Fatalf("generated doc must audit clean, got %d criticals: %s", auditOut.Meta.Audit.CriticalCount, auditOut.Answer)
	}
}

func TestMatrixVisaJustificationText(t *testing.T) {
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
