package store

import (
	"path/filepath"
	"testing"
)

func TestRegulationLifecycle(t *testing.T) {
	dir := t.TempDir()
	s, err := New(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	chunk := func(i int) RegulationChunk {
		return RegulationChunk{
			ID:           "reg1|c" + string(rune('0'+i)),
			RegulationID: "reg1",
			Seq:          i,
			Clause:       "п. 1.1",
			Title:        "Фрагмент",
			Text:         "Достаточно длинный текст фрагмента регламента для проверки хранилища и индекса знаний.",
		}
	}
	if err := s.SaveRegulation("reg1", "Инструкция по ДОУ", "Инструкция", "od184.pdf", "/tmp/od184.pdf", RegDraft,
		[]RegulationChunk{chunk(1), chunk(2)}); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveRegulation("reg2", "Старая редакция", "Инструкция", "od184-old.pdf", "/tmp/old.pdf", RegActive,
		[]RegulationChunk{{ID: "reg2|c1", RegulationID: "reg2", Seq: 1, Clause: "п. 1.1", Title: "t", Text: "старый активный текст регламента достаточно длинный для индексации и теста"}}); err != nil {
		t.Fatal(err)
	}

	list, err := s.ListRegulations()
	if err != nil || len(list) != 2 {
		t.Fatalf("list=%v err=%v", list, err)
	}

	// Activate reg1 → reg2 must be demoted to archived.
	prevIDs, targetIDs, err := s.SetRegulationStatus("reg1", RegActive)
	if err != nil {
		t.Fatal(err)
	}
	if len(prevIDs) != 1 || prevIDs[0] != "reg2|c1" {
		t.Errorf("prevIDs = %v", prevIDs)
	}
	if len(targetIDs) != 2 {
		t.Errorf("targetIDs = %v", targetIDs)
	}
	r2, _ := s.GetRegulation("reg2")
	if r2.Status != RegArchived {
		t.Errorf("reg2 status = %s, want archived", r2.Status)
	}
	r1, _ := s.GetRegulation("reg1")
	if r1.Status != RegActive {
		t.Errorf("reg1 status = %s, want active", r1.Status)
	}

	active, err := s.ActiveRegulationChunks()
	if err != nil || len(active) != 2 {
		t.Fatalf("active chunks = %v err=%v", len(active), err)
	}
	if active[0].DocumentTitle != "Инструкция по ДОУ" {
		t.Errorf("joined title = %q", active[0].DocumentTitle)
	}

	ids, err := s.RegulationChunkIDs("reg1")
	if err != nil || len(ids) != 2 {
		t.Fatalf("ids=%v err=%v", ids, err)
	}

	if err := s.DeleteRegulation("reg1"); err != nil {
		t.Fatal(err)
	}
	active, _ = s.ActiveRegulationChunks()
	if len(active) != 0 {
		t.Errorf("after delete active = %d", len(active))
	}
}
