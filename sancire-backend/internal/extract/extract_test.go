package extract

import (
	"strings"
	"testing"
)

func TestChunkTextClauses(t *testing.T) {
	text := "10.1. " + strings.Repeat("Первый пункт достаточно длинный, чтобы породить осмысленный фрагмент для цитирования в чате ассистента и в карточке источника. ", 3) +
		"\n\n10.2. " + strings.Repeat("Второй пункт говорит о сроках согласования и других процессуальных деталях документооборота. ", 4)
	chunks := ChunkText(text)
	if len(chunks) < 2 {
		t.Fatalf("expected >=2 chunks, got %d", len(chunks))
	}
	if chunks[0].Clause != "п. 10.1" {
		t.Errorf("first clause = %q", chunks[0].Clause)
	}
	for _, c := range chunks {
		if len([]rune(c.Text)) > 950 {
			t.Errorf("chunk %d too long: %d runes", c.Seq, len([]rune(c.Text)))
		}
	}
}

func TestTextTXT(t *testing.T) {
	out, err := Text("reg.md", []byte("# Заголовок\n\nТело документа."))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Тело документа") {
		t.Fatalf("out=%q", out)
	}
}

func TestTextUnsupported(t *testing.T) {
	if _, err := Text("x.xls", []byte("a")); err == nil {
		t.Fatal("expected error")
	}
}
