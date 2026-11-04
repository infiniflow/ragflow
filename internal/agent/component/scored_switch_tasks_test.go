package component

import (
	"encoding/json"
	"testing"
)

func TestParseDocumentedAnswers(t *testing.T) {
	body := []byte(`{
		"model": "scoring-model",
		"answers": {
			"route": {"type": "choice", "choice": "bikes", "probabilities": {"bikes": 0.8, "parts": 0.2}, "confidence": 0.7},
			"matches_listing": {"type": "noul", "noul": 0.84},
			"condition": {"type": "score", "score": 1.4, "legend": {"1": "Like new", "2": "Light wear"}, "probabilities": {"1": 0.3, "2": 0.7}, "confidence": 0.6},
			"mug": {"type": "box", "box": {"top": 0.25, "bottom": 0.82, "left": 0.44, "right": 0.84}, "confidence": 0.92}
		}
	}`)
	answers := parseTypedAnswers(body)
	noul := answers["matches_listing"].(map[string]any)
	if noul["noul"] != 0.84 {
		t.Fatalf("noul = %#v", noul)
	}
	box := answers["mug"].(map[string]any)["box"].(map[string]any)
	if box["left"] != 0.44 || box["top"] != 0.25 {
		t.Fatalf("box = %#v", box)
	}
	parsed, err := parseScoringPayload(body)
	if err != nil || parsed.Probabilities["bikes"] != 0.8 {
		t.Fatalf("choice = %#v %v", parsed, err)
	}
}

func TestTextOnlyModelDropsImageAndBox(t *testing.T) {
	poster := &scriptPoster{body: choicePayload(map[string]float64{"billing": 0.9, "technical": 0.1})}
	c := newTestSwitch(scoredSwitchParam{
		Tasks: []scoringTask{{
			ID:           "mug",
			Type:         "box",
			Instructions: "the coffee mug in `photo`",
		}},
	}, poster)
	c.caps = &scoringCapabilities{Known: true}
	if _, err := c.Invoke(t.Context(), nil, map[string]any{"image": tinyPNG(t)}); err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.Unmarshal(poster.got.Body, &body); err != nil {
		t.Fatal(err)
	}
	if _, ok := body["assets"]; ok {
		t.Fatal("text-only model must not send assets")
	}
	if _, ok := body["state"].([]any); ok {
		t.Fatal("text-only model must not send image parts")
	}
	questions := body["questions"].(map[string]any)
	if _, ok := questions["mug"]; ok {
		t.Fatal("text-only model must not send a box question")
	}
	if _, ok := questions["route"]; !ok {
		t.Fatal("choice question must still be sent")
	}
}
