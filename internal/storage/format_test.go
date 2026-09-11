package storage

import (
	"strings"
	"testing"

	"bookmarkhubgo/internal/model"
)

func sampleState() model.State {
	return model.State{FormatVersion: 1, DeviceID: "mac", Counter: 2,
		Groups:    []model.Group{{ID: "g-tech", Name: "技术", Revision: "1@mac"}},
		Bookmarks: []model.Bookmark{{ID: "b-go", URL: "https://go.dev/", Title: "Go 文档", GroupID: "g-tech", Tags: []string{"Go", "教程"}, Notes: "官方文档", Starred: true, Color: "blue", CreatedAt: "2026-09-06T10:00:00Z", UpdatedAt: "2026-09-06T11:00:00Z", Revision: "2@mac"}},
	}
}

func TestHTMLRoundTripPreservesRichState(t *testing.T) {
	encoded, err := EncodeHTML(sampleState())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), "NETSCAPE-Bookmark-file-1") || !strings.Contains(string(encoded), "TAGS=\"Go,教程\"") || !strings.Contains(string(encoded), "DATA-BOOKMARKHUB-STARRED=\"true\"") || !strings.Contains(string(encoded), "DATA-BOOKMARKHUB-COLOR=\"blue\"") {
		t.Fatalf("not browser bookmark HTML: %s", encoded)
	}
	decoded, err := DecodeHTML(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if len(decoded.Bookmarks) != 1 || decoded.Bookmarks[0].Notes != "官方文档" || decoded.Bookmarks[0].Tags[1] != "教程" || !decoded.Bookmarks[0].Starred || decoded.Bookmarks[0].Color != "blue" {
		t.Fatalf("unexpected round trip: %#v", decoded)
	}
}

func TestXBELRoundTrip(t *testing.T) {
	encoded, err := EncodeXBEL(sampleState())
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeXBEL(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if len(decoded.Groups) != 1 || len(decoded.Bookmarks) != 1 || decoded.Bookmarks[0].GroupID != decoded.Groups[0].ID || decoded.Bookmarks[0].Tags[0] != "Go" || !decoded.Bookmarks[0].Starred || decoded.Bookmarks[0].Color != "blue" {
		t.Fatalf("unexpected XBEL round trip: %#v", decoded)
	}
}

func TestGenericBrowserHTMLImport(t *testing.T) {
	input := `<!DOCTYPE NETSCAPE-Bookmark-file-1><DL><p><DT><H3>工作</H3><DL><p><DT><A HREF="https://example.com" TAGS="资料,待读" DATA-BOOKMARKHUB-COLOR="green">Example</A><DD>备注</DL><p></DL><p>`
	state, err := DecodeHTML([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Groups) != 1 || len(state.Bookmarks) != 1 || state.Bookmarks[0].Notes != "备注" || state.Bookmarks[0].GroupID != state.Groups[0].ID || state.Bookmarks[0].Color != "green" {
		t.Fatalf("unexpected import: %#v", state)
	}
}
