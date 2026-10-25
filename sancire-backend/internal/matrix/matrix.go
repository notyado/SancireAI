package matrix
import (
	"fmt"
	"strings"
)
// DocKind represents general type of document
type DocKind string
const (
	KindOrder     DocKind = "ПРИКАЗ"
	KindDirective DocKind = "РАСПОРЯЖЕНИЕ"
	KindLetter    DocKind = "ПИСЬМО"
	KindMemo      DocKind = "СЛУЖЕБНАЯ ЗАПИСКА"
	KindUnknown   DocKind = "НЕ ОПРЕДЕЛЕН"
)
// Department constants
const (
	DeptLegal           = "Юридическое подразделение"
	DeptDOU             = "Департамент ДОУ"
	DeptHR              = "Кадровое подразделение"
	DeptInfoSec         = "Департамент информационной безопасности"
	DeptIT              = "Департамент информационных технологий"
	DeptInternalControl = "Департамент внутреннего контроля"
	DeptIntlCooperation = "Департамент международного сотрудничества"
)
// VisaRequirement describes visa status for a department
type VisaRequirement struct {
	Department string `json:"department"`
	Required   bool   `json:"required"`
	Reason     string `json:"reason"`
	Deadline   string `json:"deadline"`
}
// MatrixRow represents a row in the Mandatory Visa Matrix
type MatrixRow struct {
	ID          string   `json:"id"`
	RowNumber   int      `json:"rowNumber"`
	Category    string   `json:"category"`
	Description string   `json:"description"`
	Legal       bool     `json:"legal"`
	DOU         bool     `json:"dou"`
	HR          bool     `json:"hr"`
	OtherDept   []string `json:"otherDept"`
	Condition   string   `json:"condition,omitempty"`
}
// RouteStep represents one stage in Chapter 10 of DOU Instruction
type RouteStep struct {
	Order       int    `json:"order"`
	Stage       string `json:"stage"`
	Actor       string `json:"actor"`
	Description string `json:"description"`
	Deadline    string `json:"deadline"`
}
// DocumentContext contains attributes needed to determine routing
type DocumentContext struct {
	Kind              DocKind  `json:"kind"`
	Title             string   `json:"title"`
	IsExternal        bool     `json:"isExternal"`
	HasISAccessChange bool     `json:"hasISAccessChange"` // Особое условие: передача доступа к ИС
	IsITProject       bool     `json:"isITProject"`       // Особое условие: ИТ-проект или вопросы ИС
	AddresseeType     string   `json:"addresseeType"`     // "state_authority", "foreign_org", "russian_entity"
	MatchedModuleID   string   `json:"matchedModuleID"`
	CustomWording     bool     `json:"customWording"`     // Изменены ли постоянные формулировки эталона
}
// VisaCalculationResult contains full calculated routing
type VisaCalculationResult struct {
	Kind            DocKind           `json:"kind"`
	MatchedRows     []MatrixRow       `json:"matchedRows"`
	RequiredVisas   []VisaRequirement `json:"requiredVisas"`
	RouteSteps      []RouteStep       `json:"routeSteps"`
	ModulePrivilege bool              `json:"modulePrivilege"`
	PrivilegeNote   string            `json:"privilegeNote,omitempty"`
	PrivilegeWarning string           `json:"privilegeWarning,omitempty"`
	AppliedRules    []string          `json:"appliedRules"` // deterministic justification of the route
	NormativeBasis  string            `json:"normativeBasis"`
}
// Standard Matrix definition (10 rows)
var StandardMatrix = []MatrixRow{
	// 1. Внутренние документы
	{
		ID:          "internal_row_1",
		RowNumber:   1,
		Category:    "Приказы",
		Description: "Приказы по рабочему календарю, нерабочим праздничным и выходным дням",
		Legal:       true,
		DOU:         true,
		HR:          true,
	},
	{
		ID:          "internal_row_2",
		RowNumber:   2,
		Category:    "Приказы",
		Description: "Приказы о приеме и передаче дел и имущества",
		Legal:       true,
		DOU:         true,
		HR:          true,
		Condition:   "Департамент ИБ визирует при изменении/прекращении прав доступа к ИС",
	},
	{
		ID:          "internal_row_3",
		RowNumber:   3,
		Category:    "Приказы",
		Description: "Приказы по информационным системам (ввод в эксплуатацию, вывод, изменение)",
		Legal:       true,
		DOU:         true,
		HR:          false,
		OtherDept:   []string{DeptInfoSec, DeptIT},
	},
	{
		ID:          "internal_row_4",
		RowNumber:   4,
		Category:    "Приказы",
		Description: "Прочие приказы по основной деятельности",
		Legal:       true,
		DOU:         true,
		HR:          false,
	},
	{
		ID:          "internal_row_5",
		RowNumber:   5,
		Category:    "Распоряжения",
		Description: "Распоряжения о материальной помощи и единовременных выплатах",
		Legal:       false,
		DOU:         true,
		HR:          true,
	},
	{
		ID:          "internal_row_6",
		RowNumber:   6,
		Category:    "Распоряжения",
		Description: "Распоряжения о создании рабочих групп, проектных комитетов, комиссий",
		Legal:       false,
		DOU:         true,
		HR:          true,
		Condition:   "Департамент ИТ визирует, если орган создается для ИТ-проекта или вопросов ИС",
	},
	{
		ID:          "internal_row_7",
		RowNumber:   7,
		Category:    "Распоряжения",
		Description: "Прочие распоряжения Организации",
		Legal:       true,
		DOU:         true,
		HR:          false,
	},
	// 2. Внешние документы (письма)
	{
		ID:          "external_row_1",
		RowNumber:   8,
		Category:    "Внешние письма",
		Description: "Письма органам государственной власти РФ (ГД, СФ, АП, Правительство, Счетная палата)",
		Legal:       true,
		DOU:         true,
		HR:          false,
		OtherDept:   []string{DeptInternalControl},
	},
	{
		ID:          "external_row_2",
		RowNumber:   9,
		Category:    "Внешние письма",
		Description: "Письма иностранным организациям, зарубежным центробанкам, международным банкам",
		Legal:       true,
		DOU:         true,
		HR:          false,
		OtherDept:   []string{DeptIntlCooperation},
	},
	{
		ID:          "external_row_3",
		RowNumber:   10,
		Category:    "Внешние письма",
		Description: "Письма российским юридическим лицам и контрагентам",
		Legal:       true,
		DOU:         true,
		HR:          false,
	},
}
// DetectDocKind analyzes document text and title to classify document
func DetectDocKind(title, content string) DocKind {
	upper := strings.ToUpper(title + " " + content)
	if strings.Contains(upper, "ПРИКАЗ") {
		return KindOrder
	}
	if strings.Contains(upper, "РАСПОРЯЖЕНИЕ") {
		return KindDirective
	}
	if strings.Contains(upper, "СЛУЖЕБНАЯ ЗАПИСКА") || strings.Contains(upper, "СЛУЖЕБКА") {
		return KindMemo
	}
	if strings.Contains(upper, "ПИСЬМО") || strings.Contains(upper, "УВАЖАЕМЫЙ") || strings.Contains(upper, "УВАЖАЕМАЯ") {
		return KindLetter
	}
	return KindUnknown
}
// CalculateVisas calculates full visa requirements using matrix and conditions
func CalculateVisas(ctx DocumentContext) VisaCalculationResult {
	res := VisaCalculationResult{
		Kind:           ctx.Kind,
		NormativeBasis: "Матрица обязательного визирования (п. 1–3) и Глава 10 Инструкции по ДОУ № ОД-184",
	}
	deptMap := make(map[string]VisaRequirement)
	// Always require DOU
	deptMap[DeptDOU] = VisaRequirement{
		Department: DeptDOU,
		Required:   true,
		Reason:     "Обязательный контроль соблюдения требований Инструкции по ДОУ (п. 10.4.1)",
		Deadline:   "До 2 рабочих дней",
	}
	var matched []MatrixRow
	normTitle := strings.ToLower(ctx.Title)
	if ctx.IsExternal || ctx.Kind == KindLetter {
		// External letters
		switch ctx.AddresseeType {
		case "foreign_org":
			matched = append(matched, StandardMatrix[8]) // Row 9
			deptMap[DeptLegal] = VisaRequirement{Department: DeptLegal, Required: true, Reason: "Правовая экспертиза внешнего письма иностранному партнеру", Deadline: "До 3 рабочих дней"}
			deptMap[DeptIntlCooperation] = VisaRequirement{Department: DeptIntlCooperation, Required: true, Reason: "Строка 9 Матрицы: согласование международных контактов", Deadline: "До 3 рабочих дней"}
		case "state_authority":
			matched = append(matched, StandardMatrix[7]) // Row 8
			deptMap[DeptLegal] = VisaRequirement{Department: DeptLegal, Required: true, Reason: "Правовая экспертиза обращения в органы государственной власти РФ", Deadline: "До 3 рабочих дней"}
			deptMap[DeptInternalControl] = VisaRequirement{Department: DeptInternalControl, Required: true, Reason: "Строка 8 Матрицы: контроль переписки с органами власти", Deadline: "До 3 рабочих дней"}
		default:
			matched = append(matched, StandardMatrix[9]) // Row 10
			deptMap[DeptLegal] = VisaRequirement{Department: DeptLegal, Required: true, Reason: "Правовая экспертиза письма российским юрлицам", Deadline: "До 3 рабочих дней"}
		}
	} else if ctx.Kind == KindOrder {
		// Orders
		if strings.Contains(normTitle, "календар") || strings.Contains(normTitle, "выходн") || strings.Contains(normTitle, "праздничн") {
			matched = append(matched, StandardMatrix[0]) // Row 1
			deptMap[DeptLegal] = VisaRequirement{Department: DeptLegal, Required: true, Reason: "Строка 1 Матрицы: рабочий календарь", Deadline: "До 3 рабочих дней"}
			deptMap[DeptHR] = VisaRequirement{Department: DeptHR, Required: true, Reason: "Строка 1 Матрицы: кадровые условия труда", Deadline: "До 3 рабочих дней"}
		} else if strings.Contains(normTitle, "прием") || strings.Contains(normTitle, "передач") || ctx.MatchedModuleID == "M-03" {
			matched = append(matched, StandardMatrix[1]) // Row 2
			deptMap[DeptLegal] = VisaRequirement{Department: DeptLegal, Required: true, Reason: "Строка 2 Матрицы: прием-передача дел и имущества", Deadline: "До 3 рабочих дней"}
			deptMap[DeptHR] = VisaRequirement{Department: DeptHR, Required: true, Reason: "Строка 2 Матрицы: кадровая проверка должностей сдатчика и приемщика", Deadline: "До 3 рабочих дней"}
			if ctx.HasISAccessChange {
				deptMap[DeptInfoSec] = VisaRequirement{
					Department: DeptInfoSec,
					Required:   true,
					Reason:     "Особое условие Строки 2 Матрицы: передача дел связана с изменением/прекращением прав доступа к ИС",
					Deadline:   "До 3 рабочих дней",
				}
			}
		} else if strings.Contains(normTitle, "информационн") || strings.Contains(normTitle, "эксплуатац") || ctx.MatchedModuleID == "M-01" {
			matched = append(matched, StandardMatrix[2]) // Row 3
			deptMap[DeptLegal] = VisaRequirement{Department: DeptLegal, Required: true, Reason: "Строка 3 Матрицы: ввод/изменение информационной системы", Deadline: "До 3 рабочих дней"}
			deptMap[DeptInfoSec] = VisaRequirement{Department: DeptInfoSec, Required: true, Reason: "Строка 3 Матрицы: оценка безопасности внедряемой ИС", Deadline: "До 3 рабочих дней"}
			deptMap[DeptIT] = VisaRequirement{Department: DeptIT, Required: true, Reason: "Строка 3 Матрицы: техническое сопровождение и эксплуатация ИС", Deadline: "До 3 рабочих дней"}
		} else {
			// Other orders (Row 4)
			matched = append(matched, StandardMatrix[3])
			deptMap[DeptLegal] = VisaRequirement{Department: DeptLegal, Required: true, Reason: "Строка 4 Матрицы: прочие приказы по основной деятельности", Deadline: "До 3 рабочих дней"}
			if ctx.MatchedModuleID == "M-02" {
				deptMap[DeptHR] = VisaRequirement{Department: DeptHR, Required: true, Reason: "Назначение ответственных за делопроизводство в подразделении", Deadline: "До 3 рабочих дней"}
			}
		}
	} else if ctx.Kind == KindDirective {
		// Directives
		if strings.Contains(normTitle, "материальн") || strings.Contains(normTitle, "выплат") || strings.Contains(normTitle, "помощ") {
			matched = append(matched, StandardMatrix[4]) // Row 5
			deptMap[DeptHR] = VisaRequirement{Department: DeptHR, Required: true, Reason: "Строка 5 Матрицы: материальные выплаты работникам", Deadline: "До 3 рабочих дней"}
		} else if strings.Contains(normTitle, "групп") || strings.Contains(normTitle, "комитет") || strings.Contains(normTitle, "комисс") ||
			ctx.MatchedModuleID == "M-06" || ctx.MatchedModuleID == "M-07" {
			matched = append(matched, StandardMatrix[5]) // Row 6
			deptMap[DeptHR] = VisaRequirement{Department: DeptHR, Required: true, Reason: "Строка 6 Матрицы: согласование состава участников органа", Deadline: "До 3 рабочих дней"}
			if ctx.IsITProject {
				deptMap[DeptIT] = VisaRequirement{
					Department: DeptIT,
					Required:   true,
					Reason:     "Особое условие Строки 6 Матрицы: орган создается для ИТ-проекта или вопросов ИС",
					Deadline:   "До 3 рабочих дней",
				}
			}
		} else {
			// Other directives (Row 7)
			matched = append(matched, StandardMatrix[6])
			deptMap[DeptLegal] = VisaRequirement{Department: DeptLegal, Required: true, Reason: "Строка 7 Матрицы: прочие распоряжения", Deadline: "До 3 рабочих дней"}
		}
	} else {
		// Default
		deptMap[DeptLegal] = VisaRequirement{Department: DeptLegal, Required: true, Reason: "Правовая экспертиза проекта", Deadline: "До 3 рабочих дней"}
	}
	res.MatchedRows = matched
	// Deterministic justification: spell out every matched Matrix row with its
	// cell values, plus every special condition that was triggered.
	for _, row := range matched {
		cells := []string{
			fmt.Sprintf("Юр. — %s", boolCell(row.Legal)),
			fmt.Sprintf("ДОУ — %s", boolCell(row.DOU)),
			fmt.Sprintf("Кадры — %s", boolCell(row.HR)),
		}
		for _, d := range row.OtherDept {
			cells = append(cells, fmt.Sprintf("%s — Да", d))
		}
		res.AppliedRules = append(res.AppliedRules,
			fmt.Sprintf("Строка %d Матрицы (%s): %s — %s.", row.RowNumber, row.Category, row.Description, strings.Join(cells, ", ")))
		if row.Condition != "" {
			// The condition is either triggered (visa present) or not — reflect
			// the evaluated state explicitly.
			triggered := false
			for _, v := range deptMap {
				if v.Reason != "" && strings.Contains(v.Reason, fmt.Sprintf("Строки %d", row.RowNumber)) && v.Required {
					triggered = true
					break
				}
			}
			if triggered {
				res.AppliedRules = append(res.AppliedRules,
					fmt.Sprintf("Особое условие Строки %d выполнено: %s — виза включена в маршрут.", row.RowNumber, row.Condition))
			} else {
				res.AppliedRules = append(res.AppliedRules,
					fmt.Sprintf("Особое условие Строки %d не выполнено: %s — виза не требуется.", row.RowNumber, row.Condition))
			}
		}
	}
	res.AppliedRules = append(res.AppliedRules,
		"Принцип совокупности (п. 3 Матрицы): если проект относится к нескольким строкам, требования применяются одновременно; подразделение, компетенция которого затрагивается проектом, включается в маршрут независимо от отдельной отметки.")

	// Convert deptMap to ordered slice
	priorityOrder := []string{DeptLegal, DeptHR, DeptInfoSec, DeptIT, DeptInternalControl, DeptIntlCooperation, DeptDOU}
	for _, dept := range priorityOrder {
		if req, ok := deptMap[dept]; ok {
			res.RequiredVisas = append(res.RequiredVisas, req)
		}
	}
	// Module Privilege & Warning
	if ctx.MatchedModuleID != "" {
		if ctx.CustomWording {
			res.ModulePrivilege = false
			res.PrivilegeWarning = "Внимание: в модульном документе изменены постоянные формулировки эталона. В соответствии с п. 1.3 Методических рекомендаций документ теряет привилегию сокращенного согласования и должен пройти полный круг визирования по Главе 10 Инструкции по ДОУ."
		} else {
			res.ModulePrivilege = true
			res.PrivilegeNote = "Проект соответствует утвержденному модулю: в соответствии с п. 1.3 Методических рекомендаций повторное полное согласование подразделениям, согласовавшим модуль, не требуется — сохраняются лингвистическая экспертиза ДОУ, визирование по Матрице и подписание."
		}
	}
	// Generate Chapter 10 sequential route
	res.RouteSteps = []RouteStep{
		{
			Order:       1,
			Stage:       "Подготовка и визирование инициатором",
			Actor:       "Исполнитель и руководитель подразделения-инициатора",
			Description: "Первичное визирование текста проекта и обосновывающих материалов",
			Deadline:    "День создания",
		},
		{
			Order:       2,
			Stage:       "Лингвистическая экспертиза ДОУ",
			Actor:       "Корректор / Специалист Департамента ДОУ",
			Description: "Проверка соответствия официально-деловому стилю и правилам оформления реквизитов",
			Deadline:    "До 2 рабочих дней",
		},
		{
			Order:       3,
			Stage:       "Обязательное визирование по Матрице",
			Actor:       formatDeptsSummary(res.RequiredVisas),
			Description: "Правовая, кадровая и профильная экспертиза в соответствии с Матрицей",
			Deadline:    "До 3–5 рабочих дней",
		},
		{
			Order:       4,
			Stage:       "Итоговый контроль ДОУ",
			Actor:       "Департамент ДОУ",
			Description: "Проверка наличия всех виз и комплектности листа согласования перед подписанием",
			Deadline:    "1 рабочий день",
		},
		{
			Order:       5,
			Stage:       "Подписание руководством",
			Actor:       "Председатель Организации / Заместитель Председателя",
			Description: "Наложение усиленной квалифицированной электронной подписи",
			Deadline:    "По графику руководства",
		},
		{
			Order:       6,
			Stage:       "Регистрация в СЭД",
			Actor:       "Департамент ДОУ",
			Description: "Присвоение регистрационного номера и даты, помещение в дело по номенклатуре",
			Deadline:    "В течение 1 рабочего дня",
		},
	}
	return res
}
func formatDeptsSummary(visas []VisaRequirement) string {
	var depts []string
	for _, v := range visas {
		if v.Department != DeptDOU {
			depts = append(depts, v.Department)
		}
	}
	if len(depts) == 0 {
		return "Согласующие подразделения"
	}
	return strings.Join(depts, ", ")
}

func boolCell(v bool) string {
	if v {
		return "Да"
	}
	return "Нет"
}
