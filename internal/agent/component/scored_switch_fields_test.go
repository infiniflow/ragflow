package component

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
)

const documentedFieldsResponse = `{
  "model": "clairic",
  "answers": {
    "invoice_number": {"type": "text", "p_null": 3.8e-10, "span": [8, 16], "guess": "INV-2291", "confidence": 1, "value": "INV-2291"},
    "due_date": {"type": "date", "p_null": 1.9e-08, "guess": "2026-10-02", "confidence": 0.9999, "value": "2026-10-02"},
    "total": {"type": "number", "p_null": 2.3e-08, "guess": 1080, "confidence": 0.9999, "value": 1080},
    "contacts": {"type": "list", "value": ["Dana Ruiz", "Priya Shah"], "spans": [[120, 129], [131, 141]], "confidence": 0.9999},
    "po_number": {"type": "text", "p_null": 1, "span": [8, 16], "guess": "INV-2291", "confidence": 0.9995, "value": null},
    "days_to_pay": {"type": "estimate", "value": 30, "mean": 29.535, "median": 30, "p10": 21, "p90": 38, "p_below": 5.9e-06, "p_above": 1.2e-05}
  },
  "truncated": false,
  "usage": {"state_tokens": 61, "image_tokens": 0, "schema_tokens": 472, "input_tokens": 655, "output_tokens": 0, "passes": 3}
}`

func allFieldCaps() scoringCapabilities {
	return scoringCapabilities{
		Known:         true,
		Fields:        true,
		FieldText:     true,
		FieldNumber:   true,
		FieldDate:     true,
		FieldList:     true,
		FieldEstimate: true,
	}
}

func documentedFieldTasks() []scoringTask {
	decimals := 2
	return []scoringTask{
		{ID: "invoice_number", Type: "text"},
		{ID: "due_date", Type: "date", Instructions: "Issue date plus the net payment days"},
		{ID: "total", Type: "number", Decimals: &decimals, Unit: "USD", Instructions: "Amount due including shipping"},
		{ID: "contacts", Type: "list", Items: "text"},
		{ID: "po_number", Type: "text", Instructions: "Purchase order number"},
		{ID: "days_to_pay", Type: "estimate", Min: 0.0, Max: 120.0, Step: 1.0, Instructions: "Days after issue until this invoice is paid"},
	}
}

func TestParseDocumentedFieldAnswers(t *testing.T) {
	answers, err := parseFieldsAnswers([]byte(documentedFieldsResponse))
	if err != nil {
		t.Fatal(err)
	}
	invoice := answers["invoice_number"].(map[string]any)
	if invoice["value"] != "INV-2291" || invoice["guess"] != "INV-2291" || invoice["confidence"] != 1.0 {
		t.Fatalf("invoice = %#v", invoice)
	}
	span := invoice["span"].([]any)
	if len(span) != 2 || span[0] != float64(8) || span[1] != float64(16) {
		t.Fatalf("span = %#v", span)
	}
	po := answers["po_number"].(map[string]any)
	if po["value"] != nil || po["p_null"] != 1.0 {
		t.Fatalf("po = %#v", po)
	}
	total := answers["total"].(map[string]any)
	if total["value"] != float64(1080) || total["type"] != "number" {
		t.Fatalf("total = %#v", total)
	}
	contacts := answers["contacts"].(map[string]any)
	values := contacts["value"].([]any)
	if len(values) != 2 || values[0] != "Dana Ruiz" || values[1] != "Priya Shah" {
		t.Fatalf("contacts = %#v", contacts)
	}
	spans := contacts["spans"].([]any)
	if len(spans) != 2 {
		t.Fatalf("spans = %#v", spans)
	}
	estimate := answers["days_to_pay"].(map[string]any)
	mean, _ := estimate["mean"].(float64)
	if estimate["value"] != float64(30) || estimate["median"] != float64(30) || math.Abs(mean-29.535) > 1e-6 {
		t.Fatalf("estimate = %#v", estimate)
	}
	if estimate["p10"] != float64(21) || estimate["p90"] != float64(38) {
		t.Fatalf("interval = %#v", estimate)
	}
	usage := parseScoringUsage([]byte(documentedFieldsResponse))
	if usage["state_tokens"] != float64(61) || usage["image_tokens"] != float64(0) || usage["schema_tokens"] != float64(472) || usage["passes"] != float64(3) {
		t.Fatalf("usage = %#v", usage)
	}
}

func TestFieldsRequestUsesDocumentedShape(t *testing.T) {
	poster := &scriptPoster{body: []byte(documentedFieldsResponse)}
	c := newTestSwitch(scoredSwitchParam{
		ModelID:       "clairic",
		Query:         "Invoice INV-2291 from Northwind Supply, issued 2026-09-02, net 30. 4 pallets at $250.00 each, shipping $80.00. Contact: Dana Ruiz, Priya Shah.",
		Tasks:         documentedFieldTasks(),
		ReferenceDate: "2026-09-02",
		Options:       nil,
	}, poster)
	c.caps = &scoringCapabilities{}
	*c.caps = allFieldCaps()
	out, err := c.Invoke(t.Context(), nil, map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	if out["chosen_id"] != "" || out["escalated"] != false {
		t.Fatalf("branch = %#v", out)
	}
	if next, ok := out["_next"].([]any); !ok || len(next) != 0 {
		t.Fatalf("next = %#v", out["_next"])
	}
	var body map[string]any
	if err := json.Unmarshal(poster.got.Body, &body); err != nil {
		t.Fatal(err)
	}
	if _, ok := body["questions"]; ok {
		t.Fatal("field request must not send questions")
	}
	if body["model"] != "clairic" {
		t.Fatalf("model = %#v", body["model"])
	}
	if body["reference_date"] != "2026-09-02" {
		t.Fatalf("reference_date = %#v", body["reference_date"])
	}
	if strings.Contains(poster.got.Endpoint, "/decisions") || strings.Contains(poster.got.Endpoint, "/systemone") {
		t.Fatalf("endpoint = %s", poster.got.Endpoint)
	}
	fields := body["fields"].(map[string]any)
	total := fields["total"].(map[string]any)
	if total["type"] != "number" || total["decimals"] != float64(2) || total["unit"] != "USD" {
		t.Fatalf("total spec = %#v", total)
	}
	contacts := fields["contacts"].(map[string]any)
	if contacts["items"] != "text" {
		t.Fatalf("contacts spec = %#v", contacts)
	}
	days := fields["days_to_pay"].(map[string]any)
	if days["type"] != "estimate" || days["min"] != float64(0) || days["max"] != float64(120) || days["step"] != float64(1) {
		t.Fatalf("estimate spec = %#v", days)
	}
	usage := out["usage"].(map[string]any)
	if usage["passes"] != float64(3) {
		t.Fatalf("usage = %#v", usage)
	}
	answers := out["answers"].(map[string]any)
	if answers["po_number"].(map[string]any)["value"] != nil {
		t.Fatal("null value was dropped")
	}
}

func TestFieldsModeDropsQuestionsAndDisabledTypes(t *testing.T) {
	poster := &scriptPoster{body: []byte(documentedFieldsResponse)}
	c := newTestSwitch(scoredSwitchParam{
		Query: "Invoice",
		Tasks: []scoringTask{
			{ID: "invoice_number", Type: "text"},
			{ID: "mug", Type: "box", Instructions: "the mug"},
			{ID: "hidden", Type: "date", Instructions: "a date"},
		},
	}, poster)
	caps := allFieldCaps()
	caps.FieldDate = false
	c.caps = &caps
	if _, err := c.Invoke(t.Context(), nil, map[string]any{"image": "https://example.test/a.png"}); err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.Unmarshal(poster.got.Body, &body); err != nil {
		t.Fatal(err)
	}
	if _, ok := body["questions"]; ok {
		t.Fatal("questions were sent")
	}
	if _, ok := body["images"]; ok {
		t.Fatal("images were sent without image support")
	}
	fields := body["fields"].(map[string]any)
	if _, ok := fields["mug"]; ok || fields["hidden"] != nil {
		t.Fatalf("fields = %#v", fields)
	}
	if _, ok := fields["invoice_number"]; !ok {
		t.Fatal("enabled text field was dropped")
	}
}

func TestQuestionsModeDoesNotSendFields(t *testing.T) {
	poster := &scriptPoster{body: choicePayload(map[string]float64{"billing": 0.9, "technical": 0.1})}
	c := newTestSwitch(scoredSwitchParam{
		Tasks: []scoringTask{{ID: "invoice_number", Type: "text"}},
	}, poster)
	c.caps = &scoringCapabilities{Known: true, Image: true}
	if _, err := c.Invoke(t.Context(), nil, nil); err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.Unmarshal(poster.got.Body, &body); err != nil {
		t.Fatal(err)
	}
	if _, ok := body["fields"]; ok {
		t.Fatal("questions mode sent fields")
	}
	if _, ok := body["questions"].(map[string]any)["route"]; !ok {
		t.Fatal("choice question missing")
	}
}

func TestFieldsRejectsEmptyStateAndTooManyImages(t *testing.T) {
	_, err := buildFieldsCall(scoringCallInput{Model: "clairic", BaseURL: "https://example.test/run"})
	if err == nil || !strings.Contains(err.Error(), "state") {
		t.Fatalf("empty err = %v", err)
	}
	images := make([]scoringImage, 5)
	for i := range images {
		images[i] = scoringImage{DataURL: "data:image/png;base64,aa", Bytes: []byte{byte(i)}}
	}
	_, err = buildFieldsCall(scoringCallInput{
		Model:   "clairic",
		BaseURL: "https://example.test/run",
		Query:   "case",
		Images:  images,
		Tasks:   []scoringTask{{ID: "note", Type: "text"}},
	})
	if err == nil || !strings.Contains(err.Error(), "4") {
		t.Fatalf("cap err = %v", err)
	}
}

func TestFieldsImageArrayAndRemoteURL(t *testing.T) {
	poster := &scriptPoster{body: []byte(`{"answers":{"note":{"type":"text","value":"ok","guess":"ok","confidence":1,"p_null":0}}}`)}
	c := newTestSwitch(scoredSwitchParam{
		Query: "see the image",
		Tasks: []scoringTask{{ID: "note", Type: "text"}},
	}, poster)
	caps := scoringCapabilities{Known: true, Fields: true, Image: true, FieldText: true}
	c.caps = &caps
	if _, err := c.Invoke(t.Context(), nil, map[string]any{"image": tinyPNG(t)}); err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.Unmarshal(poster.got.Body, &body); err != nil {
		t.Fatal(err)
	}
	images := body["images"].([]any)
	if len(images) != 1 || !strings.HasPrefix(images[0].(string), "data:image/png") {
		t.Fatalf("images = %#v", images)
	}
	if poster.got.Headers != nil {
		t.Fatalf("headers = %#v", poster.got.Headers)
	}
	if _, err := c.Invoke(t.Context(), nil, map[string]any{"image": "https://example.test/a.png"}); err == nil || !strings.Contains(err.Error(), "remote") {
		t.Fatalf("remote err = %v", err)
	}
}
