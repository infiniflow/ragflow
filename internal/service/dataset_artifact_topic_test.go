package service

import (
	"strings"
	"testing"
)

func TestAggregateWikiTopicItemsUsesMaterializedPaths(t *testing.T) {
	items := aggregateWikiTopicItems([]map[string]interface{}{
		{"topic_kwd": " 三国演义 / 人物 / 蜀汉人物 "},
		{"topic_kwd": "三国演义/人物/蜀汉人物"},
		{"topic_kwd": "三国演义/人物/曹魏人物"},
	})
	if len(items) != 2 {
		t.Fatalf("topics = %#v, want 2 complete paths", items)
	}
	byTopic := make(map[string]WikiTopicItem, len(items))
	for _, item := range items {
		byTopic[item.Topic] = item
	}
	shuhan := byTopic["三国演义/人物/蜀汉人物"]
	if shuhan.Title != "蜀汉人物" || shuhan.Slug != shuhan.Topic || shuhan.PageCount != 2 {
		t.Fatalf("蜀汉人物 item = %#v", shuhan)
	}
	caowei := byTopic["三国演义/人物/曹魏人物"]
	if caowei.Title != "曹魏人物" || caowei.PageCount != 1 {
		t.Fatalf("曹魏人物 item = %#v", caowei)
	}
}

func TestAggregateWikiTopicItemsIgnoresPathCase(t *testing.T) {
	items := aggregateWikiTopicItems([]map[string]interface{}{
		{"topic_kwd": "Knowledge/Core"},
		{"topic_kwd": "knowledge / core"},
	})
	if len(items) != 1 || items[0].PageCount != 2 {
		t.Fatalf("topics = %#v, want one case-insensitive path", items)
	}
}

func TestWikiPageItemMatchesKeyword(t *testing.T) {
	item := WikiPageItem{
		Slug:    "Daisy",
		Title:   "Daisy",
		Topic:   "General",
		Summary: "The field flower the Happy Prince met",
	}
	for _, kw := range []string{"daisy", "DAISY", "field flower"} {
		if !wikiPageItemMatchesKeyword(item, strings.ToLower(kw)) {
			t.Fatalf("keyword %q should match item %#v", kw, item)
		}
	}
	if wikiPageItemMatchesKeyword(item, "swallow") {
		t.Fatalf("keyword %q should not match item %#v", "swallow", item)
	}
	// The topic is navigation metadata, not a searchable page field.
	if wikiPageItemMatchesKeyword(item, "general") {
		t.Fatalf("keyword %q should not match item topic field %#v", "general", item)
	}
}

func TestWikiPageItemsFromChunksStripsPageTypePrefix(t *testing.T) {
	items := wikiPageItemsFromChunks([]map[string]interface{}{
		{
			"slug_kwd":            "entity/Daisy",
			"title_kwd":           "Daisy",
			"page_type_kwd":       "entity",
			"topic_kwd":           " General ",
			"summary_with_weight": "a summary",
		},
	})
	if len(items) != 1 {
		t.Fatalf("items = %#v, want 1", items)
	}
	got := items[0]
	if got.Slug != "Daisy" || got.Title != "Daisy" || got.PageType != "entity" || got.Topic != "General" || got.Summary != "a summary" {
		t.Fatalf("item = %#v, want bare slug and mapped fields", got)
	}
}

func TestFilterWikiTopicItemsByKeyword(t *testing.T) {
	items := []WikiTopicItem{
		{Topic: "General", Title: "General"},
		{Topic: "People/Writers", Title: "Writers"},
		{Topic: "Places", Title: "Places"},
	}
	// "General" holds the Daisy page; "People/Writers" matches by path; the
	// rest are dropped.
	got := filterWikiTopicItemsByKeyword(items, "writers", map[string]bool{"General": true})
	if len(got) != 2 || got[0].Topic != "General" || got[1].Topic != "People/Writers" {
		t.Fatalf("filtered = %#v, want General and People/Writers", got)
	}
}
