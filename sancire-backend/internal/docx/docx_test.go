package docx
import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)
func TestGenerateAndAuditModule01(t *testing.T) {
	templatesDir := "../../templates"
	tplPath := filepath.Join(templatesDir, "m01_is_commissioning.docx")
	origBytes, err := os.ReadFile(tplPath)
	if err != nil {
		t.Fatalf("Failed to read template %s: %v", tplPath, err)
	}
	origHash := sha256.Sum256(origBytes)
	origHashStr := hex.EncodeToString(origHash[:])
	values := map[string]string{
		"is_name":           "Автоматизированная система мониторинга платежей",
		"basis_doc":         "акта готовности от 10.10.2026 № 12",
		"modification_type": "разработанного",
		"tz_requisites":     "ТЗ от 01.02.2026 № 1",
		"start_date":        "01.11.2026",
		"customer_dept":     "Департаменту платежных систем",
		"purpose":           "проведение расчетов",
		"controller_person": "заместителя директора Департамента ИТ Сидорова С.С.",
		"signatory":         "Заместитель Председателя Банка России Иванов И.И.",
	}
	docBytes, filename, err := GenerateModuleDocx(templatesDir, "M-01", values)
	if err != nil {
		t.Fatalf("GenerateModuleDocx failed: %v", err)
	}
	if len(docBytes) == 0 {
		t.Fatalf("Generated docx is empty")
	}
	// Verify template SHA-256 remains completely untouched (AC-07)
	currBytes, _ := os.ReadFile(tplPath)
	currHash := sha256.Sum256(currBytes)
	currHashStr := hex.EncodeToString(currHash[:])
	if origHashStr != currHashStr {
		t.Errorf("Template file was mutated! Orig: %s, Current: %s", origHashStr, currHashStr)
	}
	// Audit the generated document
	report, err := AuditDocx(docBytes, filename, "M-01")
	if err != nil {
		t.Fatalf("AuditDocx failed on generated document: %v", err)
	}
	// Verify no critical bracket findings remain
	for _, f := range report.Findings {
		if strings.Contains(f.Title, "служебные сноски") {
			t.Errorf("Found uncleaned bracket placeholder in generated document: %s", f.Description)
		}
	}
	// Verify visa routing
	hasIT := false
	hasInfoSec := false
	for _, v := range report.VisaRouting.RequiredVisas {
		if strings.Contains(v.Department, "информационных технологий") {
			hasIT = true
		}
		if strings.Contains(v.Department, "информационной безопасности") {
			hasInfoSec = true
		}
	}
	if !hasIT || !hasInfoSec {
		t.Errorf("Expected IT and InfoSec visas in M-01 audit, got: %+v", report.VisaRouting.RequiredVisas)
	}
}
func TestAuditUnfilledTemplate(t *testing.T) {
	templatesDir := "../../templates"
	tplPath := filepath.Join(templatesDir, "m01_is_commissioning.docx")
	rawTemplate, err := os.ReadFile(tplPath)
	if err != nil {
		t.Fatalf("Failed to read raw template: %v", err)
	}
	report, err := AuditDocx(rawTemplate, "draft.docx", "M-01")
	if err != nil {
		t.Fatalf("AuditDocx failed on template: %v", err)
	}
	if report.Status != TrafficLightRed {
		t.Errorf("Expected raw unfilled template to have RED status, got: %s", report.Status)
	}
	if report.CriticalCount == 0 {
		t.Errorf("Expected critical findings on unfilled template")
	}
}
