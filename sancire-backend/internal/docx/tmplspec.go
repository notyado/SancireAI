package docx

import (
	"encoding/xml"
	"fmt"
	"strings"
)

// This file defines the canonical content of the seven approved module
// templates. `cmd/mktemplates` renders these specs into .docx files, and
// processDocumentXML relies on the exact structure defined here:
//   - variable fields are embedded as {{key}} tokens (single text run);
//   - footnotes [1], [2] ... are kept as in the approved paper forms and are
//     stripped from the generated project (п. 3.3 Методических рекомендаций);
//   - the approval grif, the "МОДУЛЬНЫЙ ДОКУМЕНТ" stamp and the closing notes
//     are dropped at generation time, while the registration zone and the
//     electronic signature (ЭП) zone are always preserved.

type tmplPara struct {
	Style string // "center" | "kind" | "title" | "body" | "small" | "right" | "bold" | "list"
	Text  string
}

type tmplTable struct {
	Rows [][]string
}

type tmplBlock struct {
	Kind  string // "grif" | "stamp" | "body" | "notes"
	Paras []tmplPara
	Table *tmplTable
}

func p(style, text string) tmplPara { return tmplPara{Style: style, Text: text} }

func moduleBlocks(id string) []tmplBlock {
	switch id {
	case "M-01":
		return m01Blocks()
	case "M-02":
		return m02Blocks()
	case "M-03":
		return m03Blocks()
	case "M-04":
		return m04Blocks()
	case "M-05":
		return m05Blocks()
	case "M-06":
		return m06Blocks()
	case "M-07":
		return m07Blocks()
	}
	return nil
}

// ---------------------------------------------------------------------------
// Shared blocks
// ---------------------------------------------------------------------------

func grifBlock() tmplBlock {
	return tmplBlock{
		Kind: "grif",
		Paras: []tmplPara{
			p("left", "УТВЕРЖДАЮ"),
			p("left", "Заместитель Председателя Организации"),
			p("left", "____________________ ____________________"),
			p("left", "«___» ____________ 20___ г."),
		},
	}
}

// stampBlock renders the service "МОДУЛЬНЫЙ ДОКУМЕНТ" stamp. It carries no
// module-specific wording and is dropped at generation time.
func stampBlock() tmplBlock {
	return tmplBlock{
		Kind: "stamp",
		Paras: []tmplPara{
			p("centerSmall", "МОДУЛЬНЫЙ ДОКУМЕНТ"),
			p("centerSmall", "ГЕРБ — место для изображения"),
		},
	}
}

func headBlock(kindWord string) tmplBlock {
	return tmplBlock{
		Kind: "body",
		Paras: []tmplPara{
			p("centerSmall", "НАИМЕНОВАНИЕ ОРГАНИЗАЦИИ"),
			p("kind", kindWord),
			p("regzone", "ДАТА / РЕГИСТРАЦИОННЫЙ НОМЕР (НЕ УДАЛЯТЬ)"),
			p("right", "г. Москва"),
		},
	}
}

func signEPBlock() tmplBlock {
	return tmplBlock{
		Kind: "body",
		Paras: []tmplPara{
			p("sign", "{{signatory}}"),
			p("epzone", "ВИЗУАЛИЗАЦИЯ ЭП (НЕ УДАЛЯТЬ)"),
		},
	}
}

func notesBlock(lines ...string) tmplBlock {
	paras := []tmplPara{p("bold", "Примечания по заполнению модульного документа")}
	for _, line := range lines {
		paras = append(paras, p("small", line))
	}
	return tmplBlock{Kind: "notes", Paras: paras}
}

// ---------------------------------------------------------------------------
// M-01 — Приказ о вводе в эксплуатацию информационной системы
// ---------------------------------------------------------------------------

func m01Blocks() []tmplBlock {
	return []tmplBlock{
		grifBlock(),
		stampBlock(),
		headBlock("ПРИКАЗ"),
		{
			Kind: "body",
			Paras: []tmplPara{
				p("title", "О вводе в эксплуатацию {{is_name}} [1]"),
				p("body", "На основании {{basis_doc}} [2], содержащего решение о готовности к вводу в эксплуатацию компонента информационной системы {{is_name}} (далее — «{{is_name}}»), {{modification_type}} в соответствии с {{tz_requisites}} [3],"),
				p("bold", "ПРИКАЗЫВАЮ:"),
				p("body", "1. Ввести «{{is_name}}» в эксплуатацию с {{start_date}} [4]."),
				p("body", "2. {{customer_dept}} [5] обеспечить применение «{{is_name}}» по следующему назначению: {{purpose}}."),
				p("body", "3. Департаменту информационных технологий обеспечить:"),
				p("body", "3.1. функционирование «{{is_name}}» в соответствии с эксплуатационной документацией;"),
				p("body", "3.2. сопровождение «{{is_name}}» с привлечением при необходимости внешних подрядных организаций."),
				p("body", "4. Ответственным за исполнение настоящего приказа является {{controller_person}} [6]."),
			},
		},
		signEPBlock(),
		notesBlock(
			"1. Заголовок приказа. Укажите полное наименование информационной системы либо вводимого компонента; оно должно совпадать с преамбулой и пунктами приказа.",
			"2. Документ о готовности. Укажите вид документа, дату и номер (протокол приемочных испытаний, акт приемки и иной документ о готовности к промышленной эксплуатации).",
			"3. Основание разработки. Приведите реквизиты технического задания, функциональных требований или договора.",
			"4. Дата ввода. Укажите календарную дату начала промышленной эксплуатации.",
			"5. Функциональный заказчик. Укажите полное наименование структурного подразделения — владельца бизнес-процесса.",
			"6. Ответственный за исполнение. Укажите должность в именительном падеже и фамилию с инициалами.",
			"Общие правила: не оставляйте линии для заполнения, варианты через знак «/» и номера примечаний; дату, регистрационный номер и блок визуализации электронной подписи не удаляйте.",
		),
	}
}

// ---------------------------------------------------------------------------
// M-02 — Приказ о назначении ответственных за делопроизводство
// ---------------------------------------------------------------------------

func m02Blocks() []tmplBlock {
	return []tmplBlock{
		grifBlock(),
		stampBlock(),
		headBlock("ПРИКАЗ"),
		{
			Kind: "body",
			Paras: []tmplPara{
				p("title", "О назначении ответственных за делопроизводство и документооборот"),
				p("bold", "ПРИКАЗЫВАЮ:"),
				p("body", "1. Назначить ответственными за делопроизводство и документооборот в {{department}} [1]:"),
			},
			Table: &tmplTable{
				Rows: [][]string{
					{"Роль", "Должность, инициалы и фамилия"},
					{"Основной ответственный", "{{main_responsible}} [2]"},
					{"Замещающий на период отпуска или болезни", "{{substitute_responsible}} [3]"},
				},
			},
		},
		{
			Kind: "body",
			Paras: []tmplPara{
				p("body", "2. Ответственным лицам обеспечить:"),
				p("list", "• прием, регистрацию, передачу и учет документов;"),
				p("list", "• контроль сроков исполнения и формирование дел;"),
				p("list", "• подготовку документов к передаче на архивное хранение."),
				p("body", "3. Руководителям подразделений обеспечивать замещение ответственных лиц на период их отсутствия и сообщать об изменениях в срок."),
				p("body", "4. Контроль за исполнением настоящего приказа оставляю за собой."),
			},
		},
		signEPBlock(),
		notesBlock(
			"1. Подразделение. Укажите полное официальное наименование структурного подразделения.",
			"2. Основной ответственный. Укажите должность, инициалы и фамилию работника; проверьте согласие работника и соответствие обязанностей должностным функциям.",
			"3. Замещающий. Укажите работника, замещающего основного ответственного на период отпуска или болезни.",
			"Общие правила: не оставляйте линии, необязательные скобки и номера примечаний; при необходимости дополните обязанности работой в СЭД и учетом бланков.",
		),
	}
}

// ---------------------------------------------------------------------------
// M-03 — Приказ о приеме и передаче дел и имущества
// ---------------------------------------------------------------------------

func m03Blocks() []tmplBlock {
	return []tmplBlock{
		grifBlock(),
		stampBlock(),
		headBlock("ПРИКАЗ"),
		{
			Kind: "body",
			Paras: []tmplPara{
				p("title", "О приеме и передаче дел и имущества"),
				p("body", "В связи с {{reason}} [1],"),
				p("bold", "ПРИКАЗЫВАЮ:"),
				p("body", "1. {{handing_person}} [2] сдать, а {{receiving_person}} [3] принять дела и имущество в срок до {{deadline}} [4] в порядке, предусмотренном Положением о порядке приема и передачи дел и имущества при назначении на должность и освобождении от должности руководителей структурных подразделений Организации."),
				p("body", "{{when has_is_access=Да}}Передача дел связана с предоставлением, изменением или прекращением прав доступа к информационным системам.{{/when}}"),
				p("body", "2. Контроль за исполнением настоящего приказа оставляю за собой."),
			},
		},
		signEPBlock(),
		notesBlock(
			"1. Основание. Укажите конкретное обстоятельство (назначение, освобождение от должности, перевод, реорганизацию) и при необходимости реквизиты документа-основания.",
			"2. Сдающий. Укажите должность, подразделение, инициалы и фамилию лица, обязанного сдать дела и имущество.",
			"3. Принимающий. Формулировка должна грамматически сочетаться со словом «принять».",
			"4. Срок. Укажите календарную дату завершения приема-передачи.",
			"Если при передаче изменяются или прекращаются права доступа к информационным системам, проект дополнительно визируется Департаментом информационной безопасности (Строка 2 Матрицы).",
			"Общие правила: не оставляйте в подписываемом приказе линии и номера примечаний; дату, регистрационный номер и блок визуализации электронной подписи не удаляйте.",
		),
	}
}

// ---------------------------------------------------------------------------
// M-04 — Приказ об утверждении и введении в действие локального акта
// ---------------------------------------------------------------------------

func m04Blocks() []tmplBlock {
	return []tmplBlock{
		grifBlock(),
		stampBlock(),
		headBlock("ПРИКАЗ"),
		{
			Kind: "body",
			Paras: []tmplPara{
				p("title", "Об утверждении и введении в действие {{act_name}} [1]"),
				p("body", "В целях {{goal}} [2],"),
				p("bold", "ПРИКАЗЫВАЮ:"),
				p("body", "1. Утвердить {{act_name}} [3] согласно приложению к настоящему приказу."),
				p("body", "2. Ввести указанный документ в действие с {{effective_date}} [4]."),
				p("body", "3. Обеспечить ознакомление работников, деятельность которых регулируется утверждаемым документом, и размещение актуальной редакции документа."),
				p("body", "4. Контроль за исполнением настоящего приказа оставляю за собой."),
			},
		},
		signEPBlock(),
		notesBlock(
			"1. Наименование в заголовке. Укажите вид и полное наименование положения, правил, регламента, инструкции или порядка.",
			"2. Цель. Укажите причину принятия документа: регулирование процесса, распределение полномочий или приведение локальных актов в соответствие.",
			"3. Наименование в пункте 1. Должно полностью совпадать с заголовком приказа и с наименованием утверждаемого документа в приложении.",
			"4. Дата введения. Укажите дату начала применения; при необходимости предусмотрите переходный период.",
			"Общие правила: не оставляйте в подписываемом приказе квадратные скобки, линии и номера примечаний.",
		),
	}
}

// ---------------------------------------------------------------------------
// M-05 — Распоряжение о проведении внутреннего аудита процессов
// ---------------------------------------------------------------------------

func m05Blocks() []tmplBlock {
	return []tmplBlock{
		grifBlock(),
		stampBlock(),
		headBlock("РАСПОРЯЖЕНИЕ"),
		{
			Kind: "body",
			Paras: []tmplPara{
				p("title", "О проведении внутреннего аудита процесса: {{process_name}} [1]"),
				p("body", "В целях оценки результативности процессов и управления связанными с ними рисками,"),
				p("bold", "ОБЯЗЫВАЮ:"),
				p("body", "1. Провести {{audit_period}} внутренний аудит процесса: {{process_name}} [2]."),
				p("body", "2. В ходе аудита оценить:"),
				p("list", "• соответствие установленным требованиям и внутренним документам;"),
				p("list", "• результативность контрольных процедур и управление рисками;"),
				p("list", "• полноту и достоверность документирования операций."),
				p("body", "3. Утвердить состав аудиторской группы:"),
			},
			Table: &tmplTable{
				Rows: [][]string{
					{"Роль", "Должность, подразделение, инициалы и фамилия"},
					{"Руководитель аудиторской группы", "{{audit_lead}} [3]"},
					{"Аудиторы", "{{auditors_team}} [4]"},
				},
			},
		},
		{
			Kind: "body",
			Paras: []tmplPara{
				p("body", "4. Аудиторской группе обеспечить доступ к документам, информационным системам и работникам в пределах программы аудита."),
				p("body", "5. Контроль за исполнением настоящего распоряжения оставляю за собой."),
			},
		},
		signEPBlock(),
		notesBlock(
			"1. Процесс. Укажите точное наименование проверяемого процесса так же, как в заголовке и пункте 1.",
			"2. Период. Укажите календарные даты начала и окончания аудита.",
			"3. Руководитель группы. Укажите должность, инициалы и фамилию.",
			"4. Аудиторы. Аудиторы не должны являться работниками проверяемого подразделения — это обеспечивает независимость оценки.",
			"Общие правила: не оставляйте варианты, линии и номера примечаний в подписываемом распоряжении.",
		),
	}
}

// ---------------------------------------------------------------------------
// M-06 — Распоряжение о создании проектного комитета
// ---------------------------------------------------------------------------

func m06Blocks() []tmplBlock {
	return []tmplBlock{
		grifBlock(),
		stampBlock(),
		headBlock("РАСПОРЯЖЕНИЕ"),
		{
			Kind: "body",
			Paras: []tmplPara{
				p("title", "О создании проектного комитета по проекту «{{project_name}}» [1]"),
				p("body", "В целях координации и контроля реализации проекта «{{project_name}}» [2],"),
				p("body", "{{when is_it_project=Да}}Проект связан с созданием и развитием информационных систем.{{/when}}"),
				p("bold", "ОБЯЗЫВАЮ:"),
				p("body", "1. Создать Проектный комитет по проекту «{{project_name}}» [3] (далее — Комитет) в составе:"),
			},
			Table: &tmplTable{
				Rows: [][]string{
					{"Роль в Комитете", "Должность, подразделение, инициалы и фамилия"},
					{"Председатель Комитета", "{{committee_chair}} [4]"},
					{"Члены Комитета", "по перечню, утверждаемому Председателем Комитета"},
				},
			},
		},
		{
			Kind: "body",
			Paras: []tmplPara{
				p("body", "2. Контроль за исполнением настоящего распоряжения оставляю за собой."),
			},
		},
		signEPBlock(),
		notesBlock(
			"1. Проект. Используйте единое полное наименование проекта в заголовке, преамбуле и пункте 1.",
			"2. Состав Комитета. По каждой позиции укажите точную должность, подразделение, инициалы и фамилию; неиспользуемые строки удалите.",
			"Если комитет создается для ИТ-проекта или вопросов ИС, проект дополнительно визируется Департаментом информационных технологий (Строка 6 Матрицы).",
			"Общие правила: не оставляйте в подписываемом распоряжении квадратные скобки, линии и номера примечаний.",
		),
	}
}

// ---------------------------------------------------------------------------
// M-07 — Распоряжение о создании рабочей группы
// ---------------------------------------------------------------------------

func m07Blocks() []tmplBlock {
	return []tmplBlock{
		grifBlock(),
		stampBlock(),
		headBlock("РАСПОРЯЖЕНИЕ"),
		{
			Kind: "body",
			Paras: []tmplPara{
				p("title", "О создании рабочей группы"),
				p("body", "В целях {{group_goal}} [1],"),
				p("body", "{{when is_it_project=Да}}Вопросы деятельности группы связаны с ИТ-проектами и информационными системами.{{/when}}"),
				p("bold", "ОБЯЗЫВАЮ:"),
				p("body", "1. Создать рабочую группу (далее — Рабочая группа) в составе:"),
			},
			Table: &tmplTable{
				Rows: [][]string{
					{"Роль", "Должность, подразделение, инициалы и фамилия"},
					{"Руководитель", "{{lead_name}} [2]"},
					{"Члены Рабочей группы", "работники заинтересованных подразделений"},
				},
			},
		},
		{
			Kind: "body",
			Paras: []tmplPara{
				p("body", "2. Рабочей группе до {{deadline}} [3] подготовить и представить итоговые предложения."),
				p("body", "3. Руководителю Рабочей группы организовать работу, распределить задачи и обеспечить соблюдение установленного срока."),
				p("body", "4. Контроль за исполнением настоящего распоряжения оставляю за собой."),
			},
		},
		signEPBlock(),
		notesBlock(
			"1. Цель. Укажите практическую цель: подготовка документа, анализ вопроса, выработка предложений или решение иной ограниченной задачи.",
			"2. Руководитель. Укажите должность, подразделение, инициалы и фамилию; состав группы формируйте из работников заинтересованных подразделений.",
			"3. Срок. Укажите календарную дату представления итоговых предложений.",
			"Если группа создается для ИТ-проекта или вопросов ИС, проект дополнительно визируется Департаментом информационных технологий (Строка 6 Матрицы).",
			"Общие правила: не оставляйте линии и номера примечаний; задача, срок и результат должны быть конкретными.",
		),
	}
}

// ---------------------------------------------------------------------------
// OOXML rendering
// ---------------------------------------------------------------------------

func escText(s string) string {
	var b strings.Builder
	if err := xml.EscapeText(&b, []byte(s)); err != nil {
		return s
	}
	return b.String()
}

type paraFmt struct {
	align    string
	size     string
	bold     bool
	before   string
	after    string
	indent   bool
	keepNext bool
}

func fmtFor(style string) paraFmt {
	switch style {
	case "kind":
		return paraFmt{align: "center", size: "32", bold: true, before: "120", after: "120", keepNext: true}
	case "title":
		return paraFmt{align: "center", size: "28", bold: true, before: "240", after: "240", keepNext: true}
	case "center":
		return paraFmt{align: "center", size: "28", before: "60", after: "60"}
	case "centerSmall":
		return paraFmt{align: "center", size: "22", before: "40", after: "40"}
	case "regzone":
		return paraFmt{align: "center", size: "20", bold: true, before: "80", after: "80", keepNext: true}
	case "epzone":
		return paraFmt{align: "center", size: "20", bold: true, before: "240", after: "120"}
	case "small":
		return paraFmt{size: "22", before: "40", after: "40", indent: true}
	case "right":
		return paraFmt{align: "right", size: "24", before: "40", after: "40"}
	case "sign":
		return paraFmt{align: "right", size: "28", bold: false, before: "360", after: "60"}
	case "bold":
		return paraFmt{size: "28", bold: true, before: "160", after: "80", keepNext: true}
	case "list":
		return paraFmt{size: "28", before: "20", after: "20", indent: true}
	case "left":
		return paraFmt{size: "24", before: "20", after: "20"}
	default: // "body"
		return paraFmt{size: "28", before: "60", after: "80"}
	}
}

func renderParagraph(t tmplPara) string {
	f := fmtFor(t.Style)
	var props []string
	if f.align != "" {
		props = append(props, fmt.Sprintf(`<w:jc w:val="%s"/>`, f.align))
	}
	spacing := fmt.Sprintf(`<w:spacing w:before="%s" w:after="%s"/>`, f.before, f.after)
	props = append(props, spacing)
	if f.indent {
		props = append(props, `<w:ind w:left="284" w:hanging="170"/>`)
	}
	if f.keepNext {
		props = append(props, `<w:keepNext/>`)
	}
	pPr := "<w:pPr>" + strings.Join(props, "") + "</w:pPr>"
	rProps := []string{`<w:rFonts w:ascii="Times New Roman" w:hAnsi="Times New Roman" w:cs="Times New Roman"/>`}
	if f.bold {
		rProps = append(rProps, "<w:b/>")
	}
	rProps = append(rProps, fmt.Sprintf(`<w:sz w:val="%s"/>`, f.size), fmt.Sprintf(`<w:szCs w:val="%s"/>`, f.size))
	rPr := "<w:rPr>" + strings.Join(rProps, "") + "</w:rPr>"
	return "<w:p>" + pPr + "<w:r>" + rPr + `<w:t xml:space="preserve">` + escText(t.Text) + "</w:t></w:r></w:p>"
}

func renderTable(tbl *tmplTable) string {
	if tbl == nil || len(tbl.Rows) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(`<w:tbl><w:tblPr><w:tblW w:w="0" w:type="auto"/><w:tblBorders>` +
		`<w:top w:val="single" w:sz="4" w:space="0" w:color="808080"/>` +
		`<w:left w:val="single" w:sz="4" w:space="0" w:color="808080"/>` +
		`<w:bottom w:val="single" w:sz="4" w:space="0" w:color="808080"/>` +
		`<w:right w:val="single" w:sz="4" w:space="0" w:color="808080"/>` +
		`<w:insideH w:val="single" w:sz="4" w:space="0" w:color="808080"/>` +
		`<w:insideV w:val="single" w:sz="4" w:space="0" w:color="808080"/>` +
		`</w:tblBorders></w:tblPr>`)
	for ri, row := range tbl.Rows {
		b.WriteString("<w:tr>")
		for _, cellText := range row {
			style := "body"
			if ri == 0 {
				style = "bold"
			}
			b.WriteString(`<w:tc><w:tcPr><w:tcW w:w="0" w:type="auto"/></w:tcPr>`)
			b.WriteString(renderParagraph(tmplPara{Style: style, Text: cellText}))
			b.WriteString("</w:tc>")
		}
		b.WriteString("</w:tr>")
	}
	b.WriteString("</w:tbl>")
	return b.String()
}

const docXMLPreamble = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
	`<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">`

const sectPr = `<w:sectPr><w:pgSz w:w="11906" w:h="16838"/>` +
	`<w:pgMar w:top="1134" w:right="850" w:bottom="1134" w:left="1701" w:header="708" w:footer="708" w:gutter="0"/>` +
	`</w:sectPr>`

// BuildTemplateDocumentXML renders the module template to word/document.xml.
func BuildTemplateDocumentXML(moduleID string) ([]byte, error) {
	blocks := moduleBlocks(moduleID)
	if len(blocks) == 0 {
		return nil, fmt.Errorf("no template spec for module %s", moduleID)
	}
	var b strings.Builder
	b.WriteString(docXMLPreamble)
	b.WriteString("<w:body>")
	for _, blk := range blocks {
		for _, para := range blk.Paras {
			b.WriteString(renderParagraph(para))
		}
		if blk.Table != nil {
			b.WriteString(renderTable(blk.Table))
		}
	}
	b.WriteString(sectPr)
	b.WriteString("</w:body></w:document>")
	return []byte(b.String()), nil
}

// TemplateTokens returns every field token ({{key}}) used by the module
// template. Control tokens ({{when key=value}} / {{/when}}) contribute only
// the field key they test.
func TemplateTokens(moduleID string) map[string]bool {
	tokens := make(map[string]bool)
	for _, blk := range moduleBlocks(moduleID) {
		for _, para := range blk.Paras {
			for _, tok := range extractTokens(para.Text) {
				tokens[tok] = true
			}
		}
		if blk.Table != nil {
			for _, row := range blk.Table.Rows {
				for _, cell := range row {
					for _, tok := range extractTokens(cell) {
						tokens[tok] = true
					}
				}
			}
		}
	}
	return tokens
}

// tokenKey maps one {{…}} body to the field keys it references: a plain token
// yields its key, a {{when key=value}} condition yields the tested key and the
// closing {{/when}} yields nothing.
func tokenKey(tok string) []string {
	tok = strings.TrimSpace(tok)
	switch {
	case tok == "" || strings.HasPrefix(tok, "/"):
		return nil
	case strings.HasPrefix(tok, "when "):
		cond := strings.TrimSpace(strings.TrimPrefix(tok, "when "))
		if eq := strings.Index(cond, "="); eq > 0 {
			return []string{strings.TrimSpace(cond[:eq])}
		}
		return nil
	default:
		return []string{tok}
	}
}

func extractTokens(s string) []string {
	var out []string
	rest := s
	for {
		i := strings.Index(rest, "{{")
		if i < 0 {
			break
		}
		j := strings.Index(rest[i:], "}}")
		if j < 0 {
			break
		}
		out = append(out, tokenKey(rest[i+2:i+j])...)
		rest = rest[i+j+2:]
	}
	return out
}
