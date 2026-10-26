package api

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"sancireai/backend/internal/config"
	"sancireai/backend/internal/rag"
	"sancireai/backend/internal/store"
)

func setupTestServer(t *testing.T) (http.Handler, func()) {
	tmpDB := filepath.Join(t.TempDir(), "test_api.db")
	s, err := store.New(tmpDB)
	if err != nil {
		t.Fatalf("Failed to init store: %v", err)
	}
	cfg := config.Config{
		JWTSecret:    "test-secret",
		TokenTTL:     time.Hour,
		TemplatesDir: "../../templates",
	}
	ragEngine := rag.NewEngine(rag.EngineConfig{})
	handler := NewHandler(cfg, s, ragEngine)
	router := SetupRouter(handler)
	// t.TempDir() removes the database file itself.
	return router, func() {
		s.Close()
	}
}
func TestAPI_AuthAndRBAC(t *testing.T) {
	router, teardown := setupTestServer(t)
	defer teardown()
	// 1. Login as employee
	loginPayload := `{"username":"employee","password":"employee123"}`
	req := httptest.NewRequest("POST", "/api/auth/login", bytes.NewReader([]byte(loginPayload)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("Expected 200 on login, got %d: %s", w.Code, w.Body.String())
	}
	var empAuth struct {
		Token string `json:"token"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &empAuth)
	if empAuth.Token == "" {
		t.Fatalf("Expected non-empty token")
	}
	// 2. Access admin endpoint with employee token -> 403 Forbidden (TC-09)
	reqAdmin := httptest.NewRequest("GET", "/api/admin/versions", nil)
	reqAdmin.Header.Set("Authorization", "Bearer "+empAuth.Token)
	wAdmin := httptest.NewRecorder()
	router.ServeHTTP(wAdmin, reqAdmin)
	if wAdmin.Code != http.StatusForbidden {
		t.Errorf("Expected 403 Forbidden for employee on /api/admin/versions, got %d", wAdmin.Code)
	}
	// 3. Login as admin
	adminPayload := `{"username":"admin","password":"admin123"}`
	reqAdmLogin := httptest.NewRequest("POST", "/api/auth/login", bytes.NewReader([]byte(adminPayload)))
	reqAdmLogin.Header.Set("Content-Type", "application/json")
	wAdmLogin := httptest.NewRecorder()
	router.ServeHTTP(wAdmLogin, reqAdmLogin)
	var admAuth struct {
		Token string `json:"token"`
	}
	_ = json.Unmarshal(wAdmLogin.Body.Bytes(), &admAuth)
	// 4. Access admin endpoint with admin token -> 200 OK
	reqAdminOk := httptest.NewRequest("GET", "/api/admin/versions", nil)
	reqAdminOk.Header.Set("Authorization", "Bearer "+admAuth.Token)
	wAdminOk := httptest.NewRecorder()
	router.ServeHTTP(wAdminOk, reqAdminOk)
	if wAdminOk.Code != http.StatusOK {
		t.Errorf("Expected 200 OK for admin on /api/admin/versions, got %d", wAdminOk.Code)
	}
}
func TestAPI_RegulationsAndTemplates(t *testing.T) {
	router, teardown := setupTestServer(t)
	defer teardown()
	// 1. GET /api/regulations?section=docflow-4-2
	req := httptest.NewRequest("GET", "/api/regulations?section=docflow-4-2", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("Expected 200, got %d", w.Code)
	}
	var regRes struct {
		Section struct {
			ID     string `json:"id"`
			Clause string `json:"clause"`
		} `json:"section"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &regRes)
	if regRes.Section.ID != "docflow-4-2" {
		t.Errorf("Expected docflow-4-2, got %s", regRes.Section.ID)
	}
	// 2. GET /api/templates
	reqTpl := httptest.NewRequest("GET", "/api/templates", nil)
	wTpl := httptest.NewRecorder()
	router.ServeHTTP(wTpl, reqTpl)
	if wTpl.Code != http.StatusOK {
		t.Fatalf("Expected 200 on /api/templates, got %d", wTpl.Code)
	}
}
func TestAPI_DocumentGenerateAndAudit(t *testing.T) {
	router, teardown := setupTestServer(t)
	defer teardown()
	// 1. Generate M-01 docx
	genPayload := map[string]any{
		"moduleId": "M-01",
		"values": map[string]string{
			"is_name":           "Система мониторинга платежей",
			"basis_doc":         "решения от 12.10.2026",
			"modification_type": "разработанного",
			"tz_requisites":     "ТЗ № 1",
			"start_date":        "01.11.2026",
			"customer_dept":     "Департаменту платежных систем",
			"purpose":           "проведение расчетов",
			"controller_person": "Иванова И.И.",
			"signatory":         "Заместитель Председателя Банка России",
		},
	}
	b, _ := json.Marshal(genPayload)
	reqGen := httptest.NewRequest("POST", "/api/documents/generate", bytes.NewReader(b))
	reqGen.Header.Set("Content-Type", "application/json")
	wGen := httptest.NewRecorder()
	router.ServeHTTP(wGen, reqGen)
	if wGen.Code != http.StatusOK {
		t.Fatalf("Generate document failed with code %d: %s", wGen.Code, wGen.Body.String())
	}
	generatedBytes := wGen.Body.Bytes()
	if len(generatedBytes) == 0 {
		t.Fatalf("Empty generated document bytes")
	}
	// 2. Audit generated document via multipart POST /api/documents/audit
	body := new(bytes.Buffer)
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("file", "test_order.docx")
	_, _ = part.Write(generatedBytes)
	_ = writer.WriteField("moduleId", "M-01")
	_ = writer.Close()
	reqAudit := httptest.NewRequest("POST", "/api/documents/audit", body)
	reqAudit.Header.Set("Content-Type", writer.FormDataContentType())
	wAudit := httptest.NewRecorder()
	router.ServeHTTP(wAudit, reqAudit)
	if wAudit.Code != http.StatusOK {
		t.Fatalf("Audit document failed with code %d: %s", wAudit.Code, wAudit.Body.String())
	}
	var report struct {
		Status        string `json:"status"`
		CriticalCount int    `json:"criticalCount"`
	}
	_ = json.Unmarshal(wAudit.Body.Bytes(), &report)
	if report.CriticalCount > 0 {
		t.Errorf("Clean generated document should have 0 critical findings, got %d", report.CriticalCount)
	}
}
