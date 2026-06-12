package pdfparser

import (
	"strings"
)

// ---- MergeSameBullet (Python: pdf_parser.py _merge_same_bullet) ----

// MergeSameBullet merges adjacent boxes that start with the same bullet/number
// character, combining their text with a newline separator.
func MergeSameBullet(boxes []TextBox, tok Tokenizer) []TextBox {
	if len(boxes) < 2 {
		return boxes
	}
	i := 0
	for i+1 < len(boxes) {
		b := boxes[i]
		bNext := boxes[i+1]

		// Skip empty
		if strings.TrimSpace(b.Text) == "" {
			boxes = append(boxes[:i], boxes[i+1:]...)
			continue
		}
		if strings.TrimSpace(bNext.Text) == "" {
			boxes = append(boxes[:i+1], boxes[i+2:]...)
			continue
		}

		// Get first rune of each
		firstB := firstRuneString(b.Text)
		firstBNext := firstRuneString(bNext.Text)

		// Conditions to NOT merge:
		// - different first chars
		// - first char is an English letter
		// - first char is Chinese
		// - boxes don't vertically overlap (b above bNext)
		if firstB != firstBNext ||
			isEnglishLetter(firstB) ||
			isChinese(firstB, tok) ||
			b.Top > bNext.Bottom {
			i++
			continue
		}

		// Merge: bNext absorbs b
		boxes[i+1].Text = b.Text + "\n" + bNext.Text
		boxes[i+1].X0 = minFloat(b.X0, bNext.X0)
		boxes[i+1].X1 = maxFloat(b.X1, bNext.X1)
		boxes[i+1].Top = b.Top
		boxes = append(boxes[:i], boxes[i+1:]...) // remove b
	}
	return boxes
}

// ---- Helpers ----

func firstRuneString(s string) rune {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	return []rune(s)[0]
}

func isEnglishLetter(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
}

// isChinese checks if a rune is a Chinese character (CJK Unified Ideograph).
func isChinese(r rune, tok Tokenizer) bool {
	if tok != nil {
		return strings.Contains(tok.Tag(string(r)), "n")
	}
	return (r >= 0x4E00 && r <= 0x9FFF) ||
		(r >= 0x3400 && r <= 0x4DBF) ||
		(r >= 0x20000 && r <= 0x2A6DF)
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
