package post

import (
	"context"
	"errors"
	"image"
)

// ImageDescriber describes an image using a vision language model.
type ImageDescriber interface {
	DescribeImage(ctx context.Context, img image.Image, prompt string) (string, error)
}

// maxDescribeConcurrency limits how many concurrent VLM calls are in flight.
const maxDescribeConcurrency = 10

// minImageSide is the minimum width or height (in pixels) for an image
// to be sent to a VLM.  Tiny crops fail provider image-size limits.
const minImageSide = 11

// describePrompt is the default prompt for image/table description.
// Python: vision_llm_figure_describe_prompt.md
const describePrompt = `## ROLE

You are an expert visual data analyst.

## GOAL

Analyze the image and produce a textual representation strictly based on what is visible in the image.

## DECISION RULE (CRITICAL)

First, determine whether the image contains an explicit visual data representation with enumerable data units forming a coherent dataset.

## OUTPUT RULES (STRICT)

- Produce output in exactly one of the two modes defined below.
- Do NOT mention, label, or reference the modes in the output.
- Do NOT combine content from both modes.
- Do NOT explain or justify the choice of mode.
- Do NOT add any headings, titles, or commentary beyond what the mode requires.

---

## MODE 1: STRUCTURED VISUAL DATA OUTPUT

(Use only if the image contains enumerable data units forming a coherent dataset.)

Output only the following fields, in list form:
- Visual Type:
- Title:
- Axes / Legends / Labels:
- Data Points:
- Captions / Annotations:

---

## MODE 2: GENERAL FIGURE CONTENT

(Use only if the image does NOT contain enumerable data units.)

Write the content directly, starting from the first sentence.
Do NOT add any introductory labels, titles, headings, or prefixes.

Requirements:
- Describe visible regions and components in a stable order (e.g., top-to-bottom, left-to-right).
- Explicitly name interface elements or visual objects exactly as they appear.
- Transcribe all visible text verbatim; do not paraphrase, summarize, or reinterpret labels.
- Describe spatial grouping, containment, and alignment of elements.
- Do NOT interpret intent, behavior, workflows, gameplay rules, or processes.
- Avoid narrative or stylistic language unless it is a dominant and functional visual element.

Use concise, information-dense sentences.
Do not use bullet lists or structured fields in this mode.`

// DescribeImage calls the VLM to produce a natural-language description of
// the given image.  Returns the description text or an error.
//
// Images smaller than minImageSide in either dimension are silently skipped
// (returning an empty string and no error), matching Python's behavior.
func DescribeImage(ctx context.Context, img image.Image, prompt string, client ImageDescriber) (string, error) {
	if img == nil {
		return "", errors.New("DescribeImage: nil image")
	}
	b := img.Bounds()
	if b.Dx() == 0 || b.Dy() == 0 {
		return "", errors.New("DescribeImage: empty image (0x0)")
	}
	if b.Dx() < minImageSide || b.Dy() < minImageSide {
		return "", nil // skip tiny crops, Python compatible
	}

	if err := ctx.Err(); err != nil {
		return "", err
	}

	return client.DescribeImage(ctx, img, prompt)
}
