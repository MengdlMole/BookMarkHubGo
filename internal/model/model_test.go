package model

import "testing"

func TestMergeUsesLamportRevision(t *testing.T) {
	one := State{Counter: 2, Bookmarks: []Bookmark{{ID: "b-1", Title: "old", Revision: "2@mac"}}}
	two := State{Counter: 3, Bookmarks: []Bookmark{{ID: "b-1", Title: "new", Revision: "3@windows"}}}
	merged := Merge(one, two)
	if len(merged.Bookmarks) != 1 || merged.Bookmarks[0].Title != "new" || merged.Counter != 3 {
		t.Fatalf("unexpected merge: %#v", merged)
	}
}

func TestNormalizeTags(t *testing.T) {
	tags := NormalizeTags([]string{"Go, 教程", "go，待读", "教程"})
	if len(tags) != 3 || tags[0] != "Go" || tags[2] != "待读" {
		t.Fatalf("unexpected tags: %#v", tags)
	}
}
