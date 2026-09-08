package storage

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"strings"
	"time"

	"bookmarkhubgo/internal/model"
)

func EncodeXBEL(state model.State) ([]byte, error) {
	var out bytes.Buffer
	out.WriteString("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n")
	out.WriteString("<!DOCTYPE xbel>\n<xbel version=\"1.0\"><title>BookmarkHub</title>\n")
	writeXBELLevel(&out, state, "", 1)
	out.WriteString("</xbel>\n")
	return out.Bytes(), nil
}

func writeXBELLevel(out *bytes.Buffer, state model.State, parentID string, depth int) {
	indent := strings.Repeat("  ", depth)
	for _, group := range state.Groups {
		if group.Deleted || group.ParentID != parentID {
			continue
		}
		out.WriteString(fmt.Sprintf("%s<folder id=\"%s\"><title>%s</title>\n", indent, xmlEscape(group.ID), xmlEscape(group.Name)))
		writeXBELLevel(out, state, group.ID, depth+1)
		out.WriteString(indent + "</folder>\n")
	}
	for _, bookmark := range state.Bookmarks {
		if bookmark.Deleted || bookmark.GroupID != parentID {
			continue
		}
		out.WriteString(fmt.Sprintf("%s<bookmark id=\"%s\" href=\"%s\" added=\"%s\" modified=\"%s\">\n", indent, xmlEscape(bookmark.ID), xmlEscape(bookmark.URL), xmlEscape(bookmark.CreatedAt), xmlEscape(bookmark.UpdatedAt)))
		out.WriteString(indent + "  <title>" + xmlEscape(bookmark.Title) + "</title>\n")
		if bookmark.Notes != "" {
			out.WriteString(indent + "  <desc>" + xmlEscape(bookmark.Notes) + "</desc>\n")
		}
		if len(bookmark.Tags) > 0 || bookmark.Starred {
			out.WriteString(indent + "  <info><metadata owner=\"urn:bookmarkhub:xbel\" tags=\"" + xmlEscape(strings.Join(bookmark.Tags, ",")) + "\" starred=\"" + fmt.Sprintf("%t", bookmark.Starred) + "\"/></info>\n")
		}
		out.WriteString(indent + "</bookmark>\n")
	}
}

func DecodeXBEL(content []byte) (model.State, error) {
	decoder := xml.NewDecoder(bytes.NewReader(content))
	state := model.State{FormatVersion: model.FormatVersion}
	var groups []string
	var currentGroup *model.Group
	var currentBookmark *model.Bookmark
	var captureName string
	var captureText strings.Builder
	now := time.Now().UTC().Format(time.RFC3339)

	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return model.State{}, fmt.Errorf("parse XBEL: %w", err)
		}
		switch value := token.(type) {
		case xml.StartElement:
			name := strings.ToLower(value.Name.Local)
			attrs := attributes(value.Attr)
			switch name {
			case "folder":
				parent := ""
				if len(groups) > 0 {
					parent = groups[len(groups)-1]
				}
				state.Counter++
				group := model.Group{ID: first(attrs["id"], model.NewID("g")), ParentID: parent, Order: len(state.Groups), Revision: model.Revision{Counter: state.Counter, DeviceID: "import"}.String()}
				state.Groups = append(state.Groups, group)
				currentGroup = &state.Groups[len(state.Groups)-1]
				groups = append(groups, group.ID)
			case "bookmark":
				groupID := ""
				if len(groups) > 0 {
					groupID = groups[len(groups)-1]
				}
				state.Counter++
				created := first(attrs["added"], now)
				bookmark := model.Bookmark{ID: first(attrs["id"], model.NewID("b")), URL: attrs["href"], GroupID: groupID, CreatedAt: created, UpdatedAt: first(attrs["modified"], created), Revision: model.Revision{Counter: state.Counter, DeviceID: "import"}.String()}
				state.Bookmarks = append(state.Bookmarks, bookmark)
				currentBookmark = &state.Bookmarks[len(state.Bookmarks)-1]
			case "title", "desc":
				captureName = name
				captureText.Reset()
			case "metadata":
				if currentBookmark != nil {
					if attrs["tags"] != "" {
						currentBookmark.Tags = model.NormalizeTags([]string{attrs["tags"]})
					}
					currentBookmark.Starred = attrs["starred"] == "true" || attrs["starred"] == "1"
				}
			}
		case xml.CharData:
			if captureName != "" {
				captureText.Write(value)
			}
		case xml.EndElement:
			name := strings.ToLower(value.Name.Local)
			if name == captureName {
				text := strings.TrimSpace(captureText.String())
				if captureName == "title" {
					if currentBookmark != nil {
						currentBookmark.Title = text
					} else if currentGroup != nil {
						currentGroup.Name = text
					}
				} else if captureName == "desc" && currentBookmark != nil {
					currentBookmark.Notes = text
				}
				captureName = ""
			}
			switch name {
			case "bookmark":
				if currentBookmark != nil && currentBookmark.Title == "" {
					currentBookmark.Title = currentBookmark.URL
				}
				currentBookmark = nil
			case "folder":
				if len(groups) > 0 {
					groups = groups[:len(groups)-1]
				}
				currentGroup = nil
			}
		}
	}
	return state, nil
}

func xmlEscape(value string) string {
	var out bytes.Buffer
	_ = xml.EscapeText(&out, []byte(value))
	return out.String()
}
