package store
import (
	"os"
	"testing"
)
func TestStoreBasics(t *testing.T) {
	tmpDB := "./test_sancire.db"
	defer os.Remove(tmpDB)
	s, err := New(tmpDB)
	if err != nil {
		t.Fatalf("Failed to initialize store: %v", err)
	}
	defer s.Close()
	// 1. Verify user seeding & authentication
	emp, err := s.ValidateUser("employee", "employee123")
	if err != nil || emp.Role != "Employee" {
		t.Fatalf("Failed to authenticate employee: %v", err)
	}
	adm, err := s.ValidateUser("admin", "admin123")
	if err != nil || adm.Role != "DOU_Admin" {
		t.Fatalf("Failed to authenticate admin: %v", err)
	}
	// 2. Conversation lifecycle
	conv, err := s.CreateConversation("c-001", "Тестовый диалог")
	if err != nil {
		t.Fatalf("Failed to create conversation: %v", err)
	}
	msg, err := s.CreateMessage("m-001", conv.ID, "user", "Привет, как дела?", nil)
	if err != nil {
		t.Fatalf("Failed to create message: %v", err)
	}
	msgs, err := s.ListMessages(conv.ID)
	if err != nil || len(msgs) != 1 || msgs[0].ID != msg.ID {
		t.Fatalf("Failed to list messages: %v", err)
	}
	// 3. Settings
	st, err := s.GetSettings()
	if err != nil || st.Theme != "dark" {
		t.Fatalf("Expected default dark theme, got: %+v", st)
	}
	st, err = s.UpdateSettings("light", "ru")
	if err != nil || st.Theme != "light" {
		t.Fatalf("Expected light theme after update, got: %+v", st)
	}
	// 4. Versions
	activeVer, err := s.GetActiveVersion()
	if err != nil || activeVer.Code != "v1.0" {
		t.Fatalf("Expected active version v1.0, got: %+v", activeVer)
	}
}
