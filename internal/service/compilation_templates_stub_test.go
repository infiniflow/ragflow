//go:build !ee

package service

import "testing"

func TestCommunityBuiltinTemplates(t *testing.T) {
	t.Chdir("../..")
	templates, err := loadBuiltinTemplates()
	if err != nil {
		t.Fatal(err)
	}
	if len(templates) == 0 {
		t.Fatal("shared builtin templates missing")
	}
	for _, template := range templates {
		if template.Kind == "timeline" {
			t.Fatal("enterprise timeline exposed in community palette")
		}
	}
}
