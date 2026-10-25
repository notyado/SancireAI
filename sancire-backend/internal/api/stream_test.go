package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestChatStreamPersistsExactlyOneUserAndAssistantTurn(t *testing.T) {
	router, teardown := setupTestServer(t)
	defer teardown()
	request := httptest.NewRequest(http.MethodPost, "/api/chat?stream=true", bytes.NewBufferString(`{"content":"Задайте вопрос эксперту","action":"expert"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "text/event-stream")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status=%d response=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Header().Get("Content-Type"), "text/event-stream") {
		t.Fatalf("expected SSE response, got %q", response.Header().Get("Content-Type"))
	}
	var complete map[string]any
	for _, block := range strings.Split(strings.TrimSpace(response.Body.String()), "\n\n") {
		if !strings.HasPrefix(block, "event: complete\n") {
			continue
		}
		for _, line := range strings.Split(block, "\n") {
			if strings.HasPrefix(line, "data: ") {
				if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &complete); err != nil {
					t.Fatalf("decode complete event: %v", err)
				}
			}
		}
	}
	if complete == nil {
		t.Fatalf("missing complete event: %s", response.Body.String())
	}
	messages, ok := complete["messages"].([]any)
	if !ok || len(messages) != 2 {
		t.Fatalf("expected exactly one persisted user and assistant message, got %#v", complete["messages"])
	}
	if messages[0].(map[string]any)["role"] != "user" || messages[1].(map[string]any)["role"] != "assistant" {
		t.Fatalf("unexpected message order: %#v", messages)
	}
}
