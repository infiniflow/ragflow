package tool

import (
	"fmt"
	"testing"
)

func TestReferenceIDsDistinguishURLsForSameContent(t *testing.T) {
	content := "the same snippet text"
	docA, displayA := referenceIDs("https://a.example/page", content)
	docB, displayB := referenceIDs("https://b.example/page", content)
	if docA == docB {
		t.Fatalf("documentID collided across URLs: %q", docA)
	}
	if displayA == displayB {
		t.Fatalf("displayID collided across URLs: %q", displayA)
	}
}

func TestReferenceIDsAreDeterministic(t *testing.T) {
	for _, tc := range []struct {
		name    string
		url     string
		content string
	}{
		{name: "url and content", url: "https://example.com/x", content: "body"},
		{name: "empty url", url: "", content: "body"},
		{name: "blank url", url: "   ", content: "body"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			docA, displayA := referenceIDs(tc.url, tc.content)
			docB, displayB := referenceIDs(tc.url, tc.content)
			if docA != docB || displayA != displayB {
				t.Fatalf("not deterministic: (%q,%q) vs (%q,%q)", docA, displayA, docB, displayB)
			}
		})
	}
}

func TestReferenceIDsAreUniqueAcrossInputs(t *testing.T) {
	seenDoc := make(map[string]string, 500)
	seenDisplay := make(map[string]string, 500)
	for i := 0; i < 500; i++ {
		url := fmt.Sprintf("https://example.com/%d", i)
		content := fmt.Sprintf("snippet %d", i)
		docID, displayID := referenceIDs(url, content)
		if prev, ok := seenDoc[docID]; ok {
			t.Fatalf("documentID collision: %q produced by %q and %q", docID, prev, url)
		}
		if prev, ok := seenDisplay[displayID]; ok {
			t.Fatalf("displayID collision: %q produced by %q and %q", displayID, prev, url)
		}
		seenDoc[docID] = url
		seenDisplay[displayID] = url
	}
}

func TestReferenceIDsFormat(t *testing.T) {
	docID, displayID := referenceIDs("https://example.com/page", "body")
	for name, id := range map[string]string{"documentID": docID, "displayID": displayID} {
		if len(id) != 16 {
			t.Fatalf("%s length = %d, want 16 (%q)", name, len(id), id)
		}
		for _, r := range id {
			if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
				t.Fatalf("%s = %q is not lowercase hex", name, id)
			}
		}
	}
	if docID == displayID {
		t.Fatalf("documentID and displayID must differ, both are %q", docID)
	}
}
