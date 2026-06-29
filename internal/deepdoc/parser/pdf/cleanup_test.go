package parser

import (
	"testing"
)

func TestMergeSameBullet(t *testing.T) {
	boxes := []TextBox{
		{Text: "* item 1", Top: 100, Bottom: 112, X0: 50, X1: 200},
		{Text: "* item 2", Top: 114, Bottom: 126, X0: 50, X1: 200},
	}
	result := MergeSameBullet(boxes, nil)
	if len(result) != 1 {
		t.Errorf("expected 1 merged box, got %d", len(result))
	}
}

func TestMergeSameBulletNoMerge(t *testing.T) {
	boxes := []TextBox{
		{Text: "A item", Top: 100, Bottom: 112, X0: 50, X1: 200},
		{Text: "B item", Top: 114, Bottom: 126, X0: 50, X1: 200},
	}
	result := MergeSameBullet(boxes, nil)
	if len(result) != 2 {
		t.Error("different first chars should not merge")
	}
}

func TestMergeSameBulletChinese(t *testing.T) {
	// Chinese chars start, should not merge via bullet rule
	boxes := []TextBox{
		{Text: "测试文本", Top: 100, Bottom: 112, X0: 50, X1: 200},
		{Text: "测试内容", Top: 114, Bottom: 126, X0: 50, X1: 200},
	}
	result := MergeSameBullet(boxes, nil)
	if len(result) != 2 {
		t.Error("Chinese chars should not merge via bullet rule")
	}
}
