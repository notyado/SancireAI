package docx

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"

	"sancireai/backend/internal/matrix"
)

// AuditSeverity level
type AuditSeverity string

const (
	SeverityCritical AuditSeverity = "CRITICAL" // Блокирующие ошибки
	SeverityWarning  AuditSeverity = "WARNING"  // Предупреждения
	SeverityInfo     AuditSeverity = "INFO"     // Рекомендации
)

// TrafficLight status
type TrafficLight string

const (
	TrafficLightGreen  TrafficLight = "READY"    // Готов к согласованию
	TrafficLightYellow TrafficLight = "WARNINGS" // Требует внимания
	TrafficLightRed    TrafficLight = "CRITICAL" // Критические ошибки
)
// AuditFinding represents one issue found in a document
type AuditFinding struct {
	ID             string        `json:"id"`
	Severity       AuditSeverity `json:"severity"`
	Title          string        `json:"title"`
	Description    string        `json:"description"`
	Location       string        `json:"location,omitempty"`
	RuleCitation   string        `json:"ruleCitation"`
	Recommendation string        `json:"recommendation"`
}

// AuditReport is the full report returned by the Auditor
type AuditReport struct {
	FileName        string                        `json:"fileName"`
	FileSize        int64                         `json:"fileSize"`
	Status          TrafficLight                  `json:"status"`
	StatusSummary   string                        `json:"statusSummary"`
	DetectedKind    matrix.DocKind                `json:"detectedKind"`
	DetectedModule  string                        `json:"detectedModule,omitempty"`
	MatchedModuleID string                        `json:"matchedModuleID,omitempty"`
	TotalFindings   int                           `json:"totalFindings"`
	CriticalCount   int                           `json:"criticalCount"`
	WarningCount    int                           `json:"warningCount"`
	InfoCount       int                           `json:"infoCount"`
	Findings        []AuditFinding               `json:"findings"`
	VisaRouting     matrix.VisaCalculationResult `json:"visaRouting"`
}

// ModuleDefinition provides metadata and variable parameters for a module.
// FileName, Triggers and Anchors are server internals (template path, chat
// detection phrases and the auditor's constant wording), so they are never
// serialised into GET /api/modules.
type ModuleDefinition struct {
	ID          string         `json:"id"`
	Code        string         `json:"code"`
	Title       string         `json:"title"`
	FileName    string         `json:"-"`
	Description string         `json:"description"`
	Kind        matrix.DocKind `json:"kind"`
	Fields      []ModuleField  `json:"fields"`
	Triggers    []string       `json:"-"`
	Anchors     []string       `json:"-"`
	Category    string         `json:"category,omitempty"`
}

// ModuleField defines one variable field in a modular document
type ModuleField struct {
	Key         string   `json:"key"`
	Label       string   `json:"label"`
	Question    string   `json:"question,omitempty"`    // friendly phrasing used by the chat wizard
	Placeholder string   `json:"placeholder"`           // example value shown in the form
	Type        string   `json:"type"`                  // "text", "date", "choice", "textarea"
	Required    bool     `json:"required"`
	Routing     bool     `json:"routing,omitempty"`     // affects the visa route and {{when …}} blocks, not a {{key}} token
	Options     []string `json:"options,omitempty"`
	SyncTargets []string `json:"syncTargets,omitempty"` // keys where the same value repeats
	HelperNote  string   `json:"helperNote,omitempty"`
}

var (
	modulesMu   sync.RWMutex
	moduleOrder = []string{"M-01", "M-02", "M-03", "M-04", "M-05", "M-06", "M-07"}
)

// ModuleIDs returns all module codes in stable order (M-01 … M-07 plus any dynamically registered modules).
func ModuleIDs() []string {
	modulesMu.RLock()
	defer modulesMu.RUnlock()
	res := make([]string, len(moduleOrder))
	copy(res, moduleOrder)
	return res
}

// Modules returns the definitions of every module in stable order. It backs
// GET /api/modules, so the builder form is rendered from the same registry the
// generator validates against.
func Modules() []ModuleDefinition {
	modulesMu.RLock()
	defer modulesMu.RUnlock()
	out := make([]ModuleDefinition, 0, len(moduleOrder))
	for _, id := range moduleOrder {
		if mod, ok := ModulesRegistry[id]; ok {
			out = append(out, mod)
		}
	}
	return out
}

// RegisterModule adds or updates a module definition dynamically at runtime.
func RegisterModule(mod ModuleDefinition) {
	if mod.ID == "" {
		return
	}
	modulesMu.Lock()
	defer modulesMu.Unlock()
	if _, exists := ModulesRegistry[mod.ID]; !exists {
		moduleOrder = append(moduleOrder, mod.ID)
	}
	ModulesRegistry[mod.ID] = mod
}

// LoadModulesFromFile reads a JSON file defining modules and registers them dynamically.
func LoadModulesFromFile(filePath string) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var list []ModuleDefinition
	if err := json.Unmarshal(data, &list); err != nil {
		return fmt.Errorf("parsing modules json: %w", err)
	}
	for _, mod := range list {
		RegisterModule(mod)
	}
	return nil
}
