package service

import (
	"context"
	"errors"
	"testing"

	"ragflow/internal/engine"
)

type wikiPageBodyEngine struct {
	engine.DocEngine
	getChunk func(string, string, []string) (interface{}, error)
}

func (e wikiPageBodyEngine) GetChunk(_ context.Context, index, id string, kbIDs []string) (interface{}, error) {
	return e.getChunk(index, id, kbIDs)
}

func TestLoadWikiPageBody(t *testing.T) {
	const body = "# Page\n\n[Link](entity/example)"
	lookupErr := errors.New("lookup failed")
	for _, test := range []struct {
		name, content, want string
		stored              map[string]interface{}
		err                 error
		calls               int
	}{
		{"content", body, body, nil, nil, 0},
		{"blank content", "\n\n", "\n\n", nil, nil, 0},
		{"stored Markdown", "", body, map[string]interface{}{"md_with_weight": body}, nil, 1},
		{"no Markdown column", "", "", map[string]interface{}{"content_with_weight": ""}, nil, 1},
		{"lookup error", "", "", nil, lookupErr, 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			eng := wikiPageBodyEngine{getChunk: func(index, id string, kbIDs []string) (interface{}, error) {
				calls++
				if index != "ragflow_t1" || id != "page1" || len(kbIDs) != 1 || kbIDs[0] != "kb1" {
					t.Fatalf("unexpected page lookup: %s, %s, %v", index, id, kbIDs)
				}
				return test.stored, test.err
			}}
			row := map[string]interface{}{"id": "page1", "content_with_weight": test.content}
			got, err := loadWikiPageBody(t.Context(), eng, "ragflow_t1", "kb1", row)
			if got != test.want || !errors.Is(err, test.err) || calls != test.calls {
				t.Fatalf("body = %q, calls = %d, err = %v; want %q, %d, %v", got, calls, err, test.want, test.calls, test.err)
			}
		})
	}
}
