package storage

import (
	"path/filepath"
	"testing"
)

func TestReorderGroupsAndPreserveOrderWhenEditing(t *testing.T) {
	home := t.TempDir()
	store, err := Open(home, filepath.Join(home, "sync"), 17836)
	if err != nil {
		t.Fatal(err)
	}
	a, err := store.UpsertGroup(GroupInput{Name: "A"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := store.UpsertGroup(GroupInput{Name: "B"})
	if err != nil {
		t.Fatal(err)
	}
	c, err := store.UpsertGroup(GroupInput{Name: "C"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ReorderGroups("", []string{c.ID, a.ID, b.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertGroup(GroupInput{ID: a.ID, Name: "A edited"}); err != nil {
		t.Fatal(err)
	}
	state, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Groups) != 3 || state.Groups[0].ID != c.ID || state.Groups[1].ID != a.ID || state.Groups[2].ID != b.ID {
		t.Fatalf("unexpected group order: %#v", state.Groups)
	}
	if state.Groups[1].Name != "A edited" || state.Groups[1].Order != 1 {
		t.Fatalf("editing reset group order: %#v", state.Groups[1])
	}
	if err := store.ReorderGroups("", []string{a.ID, b.ID}); err == nil {
		t.Fatal("expected incomplete sibling order to be rejected")
	}
}
