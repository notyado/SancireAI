package rag

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCallLLMStreamsIncrementalTokensAndPriorHistory(t *testing.T) {
	var request map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/x-ndjson")
		_, _ = fmt.Fprintln(w, `{"message":{"content":"Плавный "},"done":false}`)
		_, _ = fmt.Fprintln(w, `{"message":{"content":"ответ."},"done":false}`)
		_, _ = fmt.Fprintln(w, `{"message":{},"done":true}`)
	}))
	defer server.Close()

	engine := NewEngine(EngineConfig{OllamaURL: server.URL, LLMModel: "test"})
	engine.httpClient = server.Client()
	var received []string
	answer, err := engine.callLLM(context.Background(), "Текущий вопрос", []ChatMessage{
		{Role: "user", Content: "Старый вопрос"},
		{Role: "assistant", Content: "Старый ответ"},
		{Role: "user", Content: "Текущий вопрос"},
	}, func(token string) { received = append(received, token) })
	if err != nil {
		t.Fatalf("callLLM: %v", err)
	}
	if answer != "Плавный ответ." || strings.Join(received, "") != answer {
		t.Fatalf("answer=%q streamed=%q", answer, strings.Join(received, ""))
	}
	messages, ok := request["messages"].([]any)
	if !ok || len(messages) != 3 {
		t.Fatalf("expected 2 prior turns plus current prompt, got %#v", request["messages"])
	}
	last := messages[len(messages)-1].(map[string]any)
	if last["content"] != "Текущий вопрос" {
		t.Fatalf("current prompt missing from last message: %#v", last)
	}
}

func TestCallLLMReportsTruncatedStreamWithoutLosingPartialText(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		_, _ = fmt.Fprintln(w, `{"message":{"content":"Часть ответа"},"done":false}`)
	}))
	defer server.Close()
	engine := NewEngine(EngineConfig{OllamaURL: server.URL, LLMModel: "test"})
	engine.httpClient = server.Client()
	answer, err := engine.callLLM(context.Background(), "Вопрос", nil, func(string) {})
	if err == nil {
		t.Fatal("expected missing done marker to be treated as truncated stream")
	}
	if answer != "Часть ответа" {
		t.Fatalf("partial answer lost: %q", answer)
	}
}
