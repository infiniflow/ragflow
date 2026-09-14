package tool

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// ReadPythonTextMeta reads Python pipeline stage data from #@meta lines.
func ReadPythonTextMeta(pyTextDir string) ([]PyResult, error) {
	entries, err := os.ReadDir(pyTextDir)
	if err != nil {
		return nil, err
	}
	var results []PyResult
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".txt") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(pyTextDir, e.Name()))
		if err != nil {
			continue
		}
		py := PyResult{File: strings.TrimSuffix(e.Name(), ".txt"), TextLen: utf8.RuneCount(data)}
		if idx := strings.LastIndex(string(data), "\n#@meta"); idx >= 0 {
			var meta struct {
				Chars          int `json:"chars"`
				BoxesInitial   int `json:"boxes_initial"`
				BoxesTextMerge int `json:"boxes_text_merge"`
				BoxesVertMerge int `json:"boxes_vertical_merge"`
				Sections       int `json:"sections"`
			}
			if json.Unmarshal(data[idx+7:], &meta) == nil {
				py.Chars = meta.Chars
				py.BoxesInitial = meta.BoxesInitial
				py.BoxesTextMerge = meta.BoxesTextMerge
				py.BoxesVertMerge = meta.BoxesVertMerge
				py.Sections = meta.Sections
				py.Pages = 0
				py.TextLen = utf8.RuneCount(data[:idx])
			}
		}
		results = append(results, py)
	}
	return results, nil
}

// ReadGoTextMeta reads Go pipeline stage data from #@meta lines.
func ReadGoTextMeta(goTextDir string) ([]BatchResult, error) {
	entries, err := os.ReadDir(goTextDir)
	if err != nil {
		return nil, err
	}
	var results []BatchResult
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".txt") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(goTextDir, e.Name()))
		if err != nil {
			continue
		}
		r := BatchResult{
			File:    strings.TrimSuffix(e.Name(), ".txt"),
			Pages:   1,
			TextLen: utf8.RuneCount(data),
		}
		if idx := strings.LastIndex(string(data), "\n#@meta"); idx >= 0 {
			r.TextLen = utf8.RuneCount(data[:idx]) // text only, exclude #@meta
			var meta struct {
				Chars    int `json:"chars"`
				BoxesIn  int `json:"boxes_initial"`
				BoxesTM  int `json:"boxes_text_merge"`
				BoxesVM  int `json:"boxes_vertical_merge"`
				Sections int `json:"sections"`
			}
			if json.Unmarshal(data[idx+7:], &meta) == nil {
				r.Chars = meta.Chars
				r.BoxesInitial = meta.BoxesIn
				r.BoxesTextMerg = meta.BoxesTM
				r.BoxesVertMerg = meta.BoxesVM
				r.Sections = meta.Sections
			}
		}
		results = append(results, r)
	}
	return results, nil
}
