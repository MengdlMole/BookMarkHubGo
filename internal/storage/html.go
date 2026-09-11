package storage

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"html"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	"bookmarkhubgo/internal/model"
)

const metadataStart = "<!-- BOOKMARKHUB-METADATA-BASE64"

func EncodeHTML(state model.State) ([]byte, error) {
	metadata, err := json.Marshal(state)
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	out.WriteString("<!DOCTYPE NETSCAPE-Bookmark-file-1>\n")
	out.WriteString("<META HTTP-EQUIV=\"Content-Type\" CONTENT=\"text/html; charset=UTF-8\">\n")
	out.WriteString("<META NAME=\"BookmarkHub-Version\" CONTENT=\"1\">\n")
	out.WriteString("<TITLE>BookmarkHub</TITLE>\n<H1>BookmarkHub</H1>\n<DL><p>\n")
	writeHTMLLevel(&out, state, "", 1)
	out.WriteString("</DL><p>\n")
	out.WriteString(metadataStart + "\n")
	out.WriteString(base64.StdEncoding.EncodeToString(metadata))
	out.WriteString("\n-->\n")
	return out.Bytes(), nil
}

func writeHTMLLevel(out *bytes.Buffer, state model.State, parentID string, depth int) {
	indent := strings.Repeat("    ", depth)
	var groups []model.Group
	for _, group := range state.Groups {
		if !group.Deleted && group.ParentID == parentID {
			groups = append(groups, group)
		}
	}
	sort.SliceStable(groups, func(i, j int) bool {
		if groups[i].Order == groups[j].Order {
			return groups[i].Name < groups[j].Name
		}
		return groups[i].Order < groups[j].Order
	})
	for _, group := range groups {
		out.WriteString(fmt.Sprintf("%s<DT><H3 DATA-BOOKMARKHUB-ID=\"%s\" DATA-BOOKMARKHUB-REVISION=\"%s\">%s</H3>\n",
			indent, attr(group.ID), attr(group.Revision), html.EscapeString(group.Name)))
		out.WriteString(indent + "<DL><p>\n")
		writeHTMLLevel(out, state, group.ID, depth+1)
		out.WriteString(indent + "</DL><p>\n")
	}
	for _, bookmark := range state.Bookmarks {
		if bookmark.Deleted || bookmark.GroupID != parentID {
			continue
		}
		added := unixTime(bookmark.CreatedAt)
		modified := unixTime(bookmark.UpdatedAt)
		out.WriteString(fmt.Sprintf("%s<DT><A HREF=\"%s\" ADD_DATE=\"%d\" LAST_MODIFIED=\"%d\" TAGS=\"%s\" DATA-BOOKMARKHUB-ID=\"%s\" DATA-BOOKMARKHUB-REVISION=\"%s\" DATA-BOOKMARKHUB-STARRED=\"%t\" DATA-BOOKMARKHUB-COLOR=\"%s\">%s</A>\n",
			indent, attr(bookmark.URL), added, modified, attr(strings.Join(bookmark.Tags, ",")), attr(bookmark.ID), attr(bookmark.Revision), bookmark.Starred, attr(bookmark.Color), html.EscapeString(bookmark.Title)))
		if bookmark.Notes != "" {
			out.WriteString(indent + "<DD>" + html.EscapeString(bookmark.Notes) + "\n")
		}
	}
}

func DecodeHTML(content []byte) (model.State, error) {
	if state, ok := decodeMetadata(content); ok {
		return state, nil
	}
	return decodeBrowserHTML(content)
}

func decodeMetadata(content []byte) (model.State, bool) {
	text := string(content)
	start := strings.Index(text, metadataStart)
	if start < 0 {
		return model.State{}, false
	}
	start += len(metadataStart)
	end := strings.Index(text[start:], "-->")
	if end < 0 {
		return model.State{}, false
	}
	payload := strings.TrimSpace(text[start : start+end])
	decoded, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		return model.State{}, false
	}
	var state model.State
	if json.Unmarshal(decoded, &state) != nil {
		return model.State{}, false
	}
	if state.FormatVersion == 0 {
		state.FormatVersion = model.FormatVersion
	}
	return state, true
}

type capture struct {
	kind  string
	attrs map[string]string
	text  strings.Builder
}

func decodeBrowserHTML(content []byte) (model.State, error) {
	decoder := xml.NewDecoder(bytes.NewReader(content))
	decoder.Strict = false
	decoder.AutoClose = xml.HTMLAutoClose
	decoder.Entity = xml.HTMLEntity
	state := model.State{FormatVersion: model.FormatVersion}
	var groupStack []string
	var dlPush []bool
	var pendingGroup string
	var current *capture
	var lastBookmark int = -1
	now := time.Now().UTC().Format(time.RFC3339)

	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			// Netscape bookmark files commonly leave DD/DT/P elements unclosed.
			// Preserve a trailing description when the lenient XML tokenizer reaches EOF.
			if strings.Contains(err.Error(), "unexpected EOF") {
				if current != nil && current.kind == "dd" && lastBookmark >= 0 {
					state.Bookmarks[lastBookmark].Notes = strings.TrimSpace(current.text.String())
				}
				break
			}
			return model.State{}, fmt.Errorf("parse bookmark HTML: %w", err)
		}
		switch value := token.(type) {
		case xml.StartElement:
			name := strings.ToLower(value.Name.Local)
			switch name {
			case "h3", "a", "dd":
				current = &capture{kind: name, attrs: attributes(value.Attr)}
			case "dl":
				pushed := pendingGroup != ""
				if pushed {
					groupStack = append(groupStack, pendingGroup)
					pendingGroup = ""
				}
				dlPush = append(dlPush, pushed)
			}
		case xml.CharData:
			if current != nil {
				current.text.Write(value)
			}
		case xml.EndElement:
			name := strings.ToLower(value.Name.Local)
			if current != nil && name == current.kind {
				text := strings.TrimSpace(current.text.String())
				switch current.kind {
				case "h3":
					id := first(current.attrs["data-bookmarkhub-id"], model.NewID("g"))
					parent := ""
					if len(groupStack) > 0 {
						parent = groupStack[len(groupStack)-1]
					}
					state.Counter++
					revision := first(current.attrs["data-bookmarkhub-revision"], model.Revision{Counter: state.Counter, DeviceID: "import"}.String())
					state.Groups = append(state.Groups, model.Group{ID: id, Name: first(text, "Untitled"), ParentID: parent, Order: len(state.Groups), Revision: revision})
					pendingGroup = id
				case "a":
					id := first(current.attrs["data-bookmarkhub-id"], model.NewID("b"))
					groupID := ""
					if len(groupStack) > 0 {
						groupID = groupStack[len(groupStack)-1]
					}
					state.Counter++
					revision := first(current.attrs["data-bookmarkhub-revision"], model.Revision{Counter: state.Counter, DeviceID: "import"}.String())
					created := parseUnix(current.attrs["add_date"], now)
					updated := parseUnix(current.attrs["last_modified"], created)
					starred := current.attrs["data-bookmarkhub-starred"] == "true" || current.attrs["data-bookmarkhub-starred"] == "1"
					state.Bookmarks = append(state.Bookmarks, model.Bookmark{ID: id, URL: current.attrs["href"], Title: first(text, current.attrs["href"]), GroupID: groupID, Tags: model.NormalizeTags([]string{current.attrs["tags"]}), Starred: starred, Color: current.attrs["data-bookmarkhub-color"], CreatedAt: created, UpdatedAt: updated, Revision: revision})
					lastBookmark = len(state.Bookmarks) - 1
				case "dd":
					if lastBookmark >= 0 {
						state.Bookmarks[lastBookmark].Notes = text
					}
				}
				current = nil
			}
			if name == "dl" && len(dlPush) > 0 {
				pushed := dlPush[len(dlPush)-1]
				dlPush = dlPush[:len(dlPush)-1]
				if pushed && len(groupStack) > 0 {
					groupStack = groupStack[:len(groupStack)-1]
				}
			}
		}
	}
	return state, nil
}

func attributes(attrs []xml.Attr) map[string]string {
	result := map[string]string{}
	for _, item := range attrs {
		result[strings.ToLower(item.Name.Local)] = item.Value
	}
	return result
}

func attr(value string) string { return html.EscapeString(value) }

func first(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func unixTime(value string) int64 {
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Now().Unix()
	}
	return parsed.Unix()
}

func parseUnix(value, fallback string) string {
	seconds, err := strconv.ParseInt(value, 10, 64)
	if err != nil || seconds <= 0 {
		return fallback
	}
	return time.Unix(seconds, 0).UTC().Format(time.RFC3339)
}
