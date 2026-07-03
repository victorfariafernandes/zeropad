package store

import "testing"

func TestMemoryPadStore_Delete(t *testing.T) {
	s := NewMemoryPadStore()
	s.Set("notes", Pad{Content: "hello"})

	if _, ok := s.Get("notes"); !ok {
		t.Fatal("expected pad to exist after Set")
	}

	s.Delete("notes")

	if _, ok := s.Get("notes"); ok {
		t.Fatal("expected pad to be gone after Delete")
	}
}
