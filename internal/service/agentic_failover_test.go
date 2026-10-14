package service

import (
	"testing"

	"ragflow/internal/entity"
)

// TestAgenticFailoverModelIDs pins how a dialog's failover list is read.
//
// The list rides in llm_setting, which is a free-form JSON column shared with
// the sampling settings, so every shape that is not a plain list of non-empty
// strings has to degrade to "no failover" instead of reaching the resolver. The
// agent turns per request, so an unparseable value must not fail the turn —
// the primary model still serves it.
func TestAgenticFailoverModelIDs(t *testing.T) {
	for _, tc := range []struct {
		name string
		chat *entity.Chat
		want []string
	}{
		{"nil dialog", nil, nil},
		{"no llm_setting", &entity.Chat{}, nil},
		{"key absent", &entity.Chat{LLMSetting: entity.JSONMap{}}, nil},
		{
			"ordered ids preserved",
			&entity.Chat{LLMSetting: entity.JSONMap{
				"failover_llm_ids": []interface{}{"m1", "m2", "m3"},
			}},
			[]string{"m1", "m2", "m3"},
		},
		{
			// Order is the author's priority: EinoChatModel walks the chain in
			// order, so reordering here would silently change which model is
			// tried first.
			"order is not sorted or deduped",
			&entity.Chat{LLMSetting: entity.JSONMap{
				"failover_llm_ids": []interface{}{"z", "a", "z"},
			}},
			[]string{"z", "a", "z"},
		},
		// JSON round-trips a []string through []interface{}; a non-empty
		// []string in the map is a programming error, not user input.
		{"empty list", &entity.Chat{LLMSetting: entity.JSONMap{
			"failover_llm_ids": []interface{}{},
		}}, nil},
		{"wrong type is ignored", &entity.Chat{LLMSetting: entity.JSONMap{
			"failover_llm_ids": "m1",
		}}, nil},
		{"null is ignored", &entity.Chat{LLMSetting: entity.JSONMap{
			"failover_llm_ids": nil,
		}}, nil},
		// Blank and non-string entries are dropped, not passed to the
		// resolver: an empty id would resolve the tenant DEFAULT model and
		// silently append a model the author never chose.
		{"blank and non-string entries dropped", &entity.Chat{LLMSetting: entity.JSONMap{
			"failover_llm_ids": []interface{}{"m1", "", nil, 42, "m2"},
		}}, []string{"m1", "m2"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := agenticFailoverModelIDs(tc.chat)
			if len(got) != len(tc.want) {
				t.Fatalf("agenticFailoverModelIDs() = %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("agenticFailoverModelIDs() = %v, want %v", got, tc.want)
				}
			}
		})
	}
}
