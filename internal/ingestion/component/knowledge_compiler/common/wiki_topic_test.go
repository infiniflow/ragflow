package common

import "testing"

func TestNormalizeWikiTopicPath(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "flat", input: "蜀汉人物", want: "蜀汉人物"},
		{name: "nested", input: " 三国演义 / 人物 / 蜀汉人物 ", want: "三国演义/人物/蜀汉人物"},
		{name: "empty segments", input: "三国演义//人物/", want: "三国演义/人物"},
		{name: "spaces", input: "Three   Kingdoms / Major  Figures", want: "Three Kingdoms/Major Figures"},
		{name: "depth", input: "History/China/Three Kingdoms/People", want: "History/China/Three Kingdoms/People"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NormalizeWikiTopicPath(tt.input); got != tt.want {
				t.Fatalf("NormalizeWikiTopicPath(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestWikiTopicLeaf(t *testing.T) {
	if got := WikiTopicLeaf("三国演义 / 人物 / 蜀汉人物"); got != "蜀汉人物" {
		t.Fatalf("WikiTopicLeaf() = %q, want %q", got, "蜀汉人物")
	}
}
