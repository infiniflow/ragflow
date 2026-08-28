package service

import "testing"

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
