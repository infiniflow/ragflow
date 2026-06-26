package parser

import (
	"context"
	"image"
	"regexp"
	"strings"
	"sync"
)

// headerFooterPattern matches layout types that should be treated as
// page furniture (Python: r"(header|footer|number)" in parser.py:637).
var headerFooterPattern = regexp.MustCompile(`(header|footer|number|reference)`)

// ── step implementations ───────────────────────────────────────────────

// NormalizeLayoutType trims whitespace from LayoutType and defaults empty
// values to "text".  Matches Python's layout_type normalization in parser.py.
func NormalizeLayoutType(boxes Boxes) Boxes {
	for i := range boxes {
		lt := strings.TrimSpace(boxes[i].LayoutType)
		if lt == "" {
			lt = "text"
		}
		boxes[i].LayoutType = lt
	}
	return boxes
}

// FilterHeaderFooter removes boxes whose LayoutType matches
// header/footer/number/reference.  Python: remove_header_footer config.
func FilterHeaderFooter(boxes Boxes) Boxes {
	if len(boxes) == 0 {
		return boxes
	}
	result := make(Boxes, 0, len(boxes))
	for _, b := range boxes {
		if headerFooterPattern.MatchString(strings.TrimSpace(b.LayoutType)) {
			continue
		}
		result = append(result, b)
	}
	return result
}

// AssignDocTypeKwd sets DocTypeKwd based on LayoutType and Image presence.
// Mapping (Python: parser.py:639-648):
//   table                → "table"
//   figure               → "image"
//   no layout + has image → "image"
//   everything else       → "text"
func AssignDocTypeKwd(boxes Boxes) Boxes {
	for i := range boxes {
		lt := strings.TrimSpace(boxes[i].LayoutType)
		switch lt {
		case "table":
			boxes[i].DocTypeKwd = "table"
		case "figure":
			boxes[i].DocTypeKwd = "image"
		default:
			if lt == "" && boxes[i].Image != nil {
				boxes[i].DocTypeKwd = "image"
			} else {
				boxes[i].DocTypeKwd = "text"
			}
		}
	}
	return boxes
}

// FlattenMediaToText sets all DocTypeKwd to "text", treating tables and
// figures as plain text.  Python: flatten_media_to_text config.
func FlattenMediaToText(boxes Boxes) Boxes {
	for i := range boxes {
		boxes[i].DocTypeKwd = "text"
	}
	return boxes
}

// EnhanceWithVision adds VLM-generated descriptions to image/table boxes.
// describer may be nil, in which case this step is a no-op.
func EnhanceWithVision(ctx context.Context, boxes Boxes, describer ImageDescriber) Boxes {
	if describer == nil {
		return boxes
	}
	if len(boxes) == 0 {
		return boxes
	}

	sem := make(chan struct{}, maxDescribeConcurrency)
	var wg sync.WaitGroup

	for i := range boxes {
		b := &boxes[i]
		if b.DocTypeKwd != "table" && b.DocTypeKwd != "image" {
			continue
		}
		if b.Image == nil {
			continue
		}

		wg.Add(1)
		sem <- struct{}{}
		go func(idx int, img image.Image, origText string) {
			defer wg.Done()
			defer func() { <-sem }()

			if err := ctx.Err(); err != nil {
				return
			}

			desc, err := DescribeImage(ctx, img, describePrompt, describer)
			if err != nil || desc == "" {
				return
			}

			if origText != "" {
				boxes[idx].Text = origText + "\n" + desc
			} else {
				boxes[idx].Text = desc
			}
		}(i, b.Image, b.Text)
	}
	wg.Wait()

	return boxes
}

// Compose chains StepFn functions, returning a StepFn that applies
// each in order.  An empty list returns the identity step.
func Compose(steps ...StepFn) StepFn {
	return func(boxes Boxes) Boxes {
		for _, fn := range steps {
			boxes = fn(boxes)
		}
		return boxes
	}
}
