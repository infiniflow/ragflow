package component

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/png"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ragflow/internal/agent/runtime"
)

func boolPtr(v bool) *bool        { return &v }
func floatPtr(v float64) *float64 { return &v }

func sampleOptions() []scoredOption {
	return []scoredOption{
		{ID: "opt-a", Label: "billing", Description: "Payments", To: []string{"node-a"}},
		{ID: "opt-b", Label: "technical", Description: "Bugs", To: []string{"node-b"}},
	}
}

func choicePayload(probs map[string]float64) []byte {
	body, err := json.Marshal(map[string]any{
		"model": "scoring-model",
		"answers": map[string]any{
			"route": map[string]any{
				"type":          "choice",
				"choice":        "billing",
				"probabilities": probs,
				"confidence":    0.81,
			},
		},
	})
	if err != nil {
		panic(err)
	}
	return body
}

type scriptPoster struct {
	status int
	body   []byte
	err    error
	calls  int
	got    scoringCall
}

func (s *scriptPoster) Post(_ context.Context, call scoringCall) ([]byte, int, error) {
	s.calls++
	s.got = call
	if s.err != nil {
		return nil, 0, s.err
	}
	status := s.status
	if status == 0 {
		status = http.StatusOK
	}
	return s.body, status, nil
}

func newTestSwitch(p scoredSwitchParam, poster scoringPoster) *ScoredSwitchComponent {
	if p.ModelID == "" {
		p.ModelID = "scoring-model"
	}
	if p.Driver == "" {
		p.Driver = "openai"
	}
	if p.APIKey == "" {
		p.APIKey = "test-key"
	}
	if p.BaseURL == "" {
		p.BaseURL = "https://example.test/api/v1"
	}
	if p.Query == "" {
		p.Query = "Where does this case go?"
	}
	if len(p.Options) == 0 {
		p.Options = sampleOptions()
	}
	c := NewScoredSwitchComponent(p)
	c.poster = poster
	return c
}

func TestParseScoringPayloadOfficialFixture(t *testing.T) {
	body := []byte(`{
		"model": "scoring-model",
		"answers": {
			"department": {
				"type": "choice",
				"choice": "billing",
				"probabilities": {"billing": 0.88, "technical": 0.12, "sales": 0.0},
				"confidence": 0.81
			}
		},
		"usage": {"input_tokens": 318, "output_tokens": 34}
	}`)
	parsed, err := parseScoringPayload(body)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Probabilities["billing"] != 0.88 || parsed.Probabilities["technical"] != 0.12 || parsed.Probabilities["sales"] != 0 {
		t.Fatalf("probabilities = %#v", parsed.Probabilities)
	}
	if !parsed.HasConfidence || parsed.Confidence != 0.81 {
		t.Fatalf("confidence = %v %v", parsed.HasConfidence, parsed.Confidence)
	}
}

func TestParseScoringPayloadOpenRouterFixture(t *testing.T) {
	body := []byte(`{
		"model": "cloudflare/clef-flash",
		"answers": {
			"color": {
				"type": "choice",
				"choice": "red",
				"probabilities": {"red": 0.9844, "green": 0.008, "blue": 0.0076},
				"confidence": 0.9536
			}
		}
	}`)
	parsed, err := parseScoringPayload(body)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(parsed.Probabilities["red"]-0.9844) > 1e-9 {
		t.Fatalf("red = %v", parsed.Probabilities["red"])
	}
}

func TestParseScoringPayloadChatEnvelope(t *testing.T) {
	inner := `{"answers":{"route":{"type":"choice","probabilities":{"billing":0.7,"technical":0.3}}}}`
	body, _ := json.Marshal(map[string]any{
		"choices": []any{map[string]any{"message": map[string]any{"content": "```json\n" + inner + "\n```"}}},
	})
	parsed, err := parseScoringPayload(body)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Probabilities["billing"] != 0.7 {
		t.Fatalf("billing = %v", parsed.Probabilities["billing"])
	}
}

func TestParseScoringPayloadMalformed(t *testing.T) {
	cases := []string{
		`not-json`,
		`{"answers":{}}`,
		`{"answers":{"route":{"type":"choice","choice":"billing"}}}`,
		`{"answers":{"route":{"type":"noul","noul":0.4}}}`,
	}
	for _, body := range cases {
		if _, err := parseScoringPayload([]byte(body)); err == nil {
			t.Fatalf("expected error for %s", body)
		}
	}
}

func TestAlignAndNormalizePreservesDistribution(t *testing.T) {
	options := append(sampleOptions(), scoredOption{ID: "opt-c", Label: "sales"})
	raw := map[string]float64{"billing": 0.88, "technical": 0.12, "sales": 0}
	got, err := alignAndNormalize(options, raw)
	if err != nil {
		t.Fatal(err)
	}
	if got["billing"] != 0.88 || got["technical"] != 0.12 || got["sales"] != 0 {
		t.Fatalf("normalized official probabilities: %#v", got)
	}
}

func TestAlignAndNormalizeRawScores(t *testing.T) {
	got, err := alignAndNormalize(sampleOptions(), map[string]float64{"billing": 4, "technical": 1})
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(got["billing"]-0.8) > 1e-9 || math.Abs(got["technical"]-0.2) > 1e-9 {
		t.Fatalf("scores = %#v", got)
	}
}

func TestAlignAndNormalizeRejectsMissingOptions(t *testing.T) {
	if _, err := alignAndNormalize(sampleOptions(), map[string]float64{"other": 1}); err == nil {
		t.Fatal("expected error")
	}
}

func TestRouteEnabledMarginEscalates(t *testing.T) {
	decision, err := routeScoredOptions(sampleOptions(), map[string]float64{"billing": 0.51, "technical": 0.49}, routePolicy{
		EscalateEnabled: true,
		MarginThreshold: defaultMarginThreshold,
		EscalateTo:      []string{"node-esc"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !decision.Escalated || decision.Handle != scoredSwitchEscalateHandle || decision.ChosenID != "" || decision.ChosenLabel != "" {
		t.Fatalf("decision = %#v", decision)
	}
	if len(decision.Next) != 1 || decision.Next[0] != "node-esc" {
		t.Fatalf("next = %#v", decision.Next)
	}
	if math.Abs(decision.Margin-0.02) > 1e-9 {
		t.Fatalf("margin = %v", decision.Margin)
	}
}

func TestRouteErrorsWhenTargetMissing(t *testing.T) {
	_, err := routeScoredOptions(sampleOptions(), map[string]float64{"billing": 0.51, "technical": 0.49}, routePolicy{
		EscalateEnabled: true,
		MarginThreshold: defaultMarginThreshold,
	})
	if err == nil || !strings.Contains(err.Error(), "escalate") {
		t.Fatalf("escalate err = %v", err)
	}
	disconnected := sampleOptions()
	disconnected[0].To = nil
	_, err = routeScoredOptions(disconnected, map[string]float64{"billing": 0.9, "technical": 0.1}, routePolicy{
		EscalateEnabled: false,
	})
	if err == nil || !strings.Contains(err.Error(), "billing") {
		t.Fatalf("option err = %v", err)
	}
}

func TestRouteEnabledTieEscalates(t *testing.T) {
	decision, err := routeScoredOptions(sampleOptions(), map[string]float64{"billing": 0.5, "technical": 0.5}, routePolicy{
		EscalateEnabled: true,
		MarginThreshold: 0,
		EscalateTo:      []string{"node-esc"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !decision.Escalated || decision.Handle != scoredSwitchEscalateHandle {
		t.Fatalf("decision = %+v", decision)
	}
}

func TestRouteEnabledMinScoreEscalates(t *testing.T) {
	decision, err := routeScoredOptions(sampleOptions(), map[string]float64{"billing": 0.62, "technical": 0.38}, routePolicy{
		EscalateEnabled: true,
		MarginThreshold: 0,
		MinTopScore:     0.7,
		EscalateTo:      []string{"node-esc"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !decision.Escalated {
		t.Fatal("expected escalation")
	}
}

func TestRouteDisabledTiePicksEarliest(t *testing.T) {
	options := []scoredOption{
		{ID: "opt-a", Label: "billing", To: []string{"node-a"}},
		{ID: "opt-b", Label: "technical", To: []string{"node-b"}},
		{ID: "opt-c", Label: "sales", To: []string{"node-c"}},
	}
	decision, err := routeScoredOptions(options, map[string]float64{"billing": 0.4, "technical": 0.4, "sales": 0.2}, routePolicy{
		EscalateEnabled: false,
		MarginThreshold: 0,
		EscalateTo:      []string{"node-esc"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Escalated || decision.Handle == scoredSwitchEscalateHandle || decision.ChosenID != "opt-a" || decision.ChosenLabel != "billing" {
		t.Fatalf("decision = %+v", decision)
	}
	if len(decision.Next) != 1 || decision.Next[0] != "node-a" {
		t.Fatalf("next = %#v", decision.Next)
	}
	if decision.Scores["sales"] != 0.2 {
		t.Fatalf("scores = %#v", decision.Scores)
	}
}

func TestRouteDisabledTinyMarginPicksWinner(t *testing.T) {
	decision, err := routeScoredOptions(sampleOptions(), map[string]float64{"billing": 0.51, "technical": 0.49}, routePolicy{
		EscalateEnabled: false,
		MarginThreshold: 0.9,
		MinTopScore:     0.99,
		EscalateTo:      []string{"node-esc"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Escalated || decision.ChosenLabel != "billing" || decision.Handle != "opt-a" {
		t.Fatalf("decision = %+v", decision)
	}
	if len(decision.Next) != 1 || decision.Next[0] != "node-a" {
		t.Fatalf("next = %#v", decision.Next)
	}
}

func TestRouteSingleWinnerAndEqualMargin(t *testing.T) {
	decision, err := routeScoredOptions(sampleOptions(), map[string]float64{"billing": 0.88, "technical": 0.12}, routePolicy{
		EscalateEnabled: true,
		MarginThreshold: defaultMarginThreshold,
	})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Escalated || decision.ChosenLabel != "billing" {
		t.Fatalf("decision = %+v", decision)
	}
	tiedMargin, err := routeScoredOptions(sampleOptions(), map[string]float64{"billing": 0.6, "technical": 0.4}, routePolicy{
		EscalateEnabled: true,
		MarginThreshold: 0.2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if tiedMargin.Escalated {
		t.Fatal("margin equal to the threshold must not escalate")
	}
}

func TestScoredSwitchDefaults(t *testing.T) {
	comp, err := New(componentNameScoredSwitch, map[string]any{
		"llm_id": "scoring-model",
		"options": []any{
			map[string]any{"name": "billing", "uuid": "opt-a"},
			map[string]any{"name": "technical", "uuid": "opt-b"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	got := comp.(*ScoredSwitchComponent)
	if got.Name() != componentNameScoredSwitch {
		t.Fatalf("name = %s", got.Name())
	}
	policy := got.param.routePolicy()
	if !policy.EscalateEnabled || policy.MarginThreshold != defaultMarginThreshold || policy.MinTopScore != 0 {
		t.Fatalf("policy = %+v", policy)
	}
	format, err := normalizeRequestFormat(got.param.RequestFormat)
	if err != nil || format != requestFormatContentParts {
		t.Fatalf("format = %s err=%v", format, err)
	}
}

func TestScoringEndpoint(t *testing.T) {
	cases := []struct {
		base   string
		format string
		url    string
		wire   string
	}{
		{"https://example.test/api/v1", "", "https://example.test/api/alpha/decisions", requestFormatContentParts},
		{"https://example.test/api/v1/chat/completions", requestFormatAssets, "https://example.test/api/v1/systemone", requestFormatAssets},
		{"https://example.test/api/v1/systemone", requestFormatContentParts, "https://example.test/api/v1/systemone", requestFormatAssets},
		{"https://example.test/api/alpha/decisions", requestFormatAssets, "https://example.test/api/alpha/decisions", requestFormatContentParts},
	}
	for _, tc := range cases {
		gotURL, gotWire, err := scoringEndpoint(tc.base, tc.format)
		if err != nil {
			t.Fatal(err)
		}
		if gotURL != tc.url || gotWire != tc.wire {
			t.Fatalf("base %s format %s => %s %s, want %s %s", tc.base, tc.format, gotURL, gotWire, tc.url, tc.wire)
		}
	}
}

func TestScoredSwitchInvokeEnabledMargin(t *testing.T) {
	poster := &scriptPoster{body: choicePayload(map[string]float64{"billing": 0.51, "technical": 0.49})}
	c := newTestSwitch(scoredSwitchParam{EscalateTo: []string{"node-esc"}}, poster)
	out, err := c.Invoke(t.Context(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if poster.calls != 1 {
		t.Fatalf("calls = %d", poster.calls)
	}
	if out["escalated"] != true || out["chosen_id"] != "" || out["chosen_label"] != "" || out["selected_handle"] != scoredSwitchEscalateHandle {
		t.Fatalf("out = %#v", out)
	}
	next, _ := out["_next"].([]any)
	if len(next) != 1 || next[0] != "node-esc" {
		t.Fatalf("next = %#v", next)
	}
	var body map[string]any
	if err := json.Unmarshal(poster.got.Body, &body); err != nil {
		t.Fatal(err)
	}
	if _, ok := body["temperature"]; ok {
		t.Fatal("temperature must not be sent")
	}
	if poster.got.Endpoint != "https://example.test/api/alpha/decisions" {
		t.Fatalf("endpoint = %s", poster.got.Endpoint)
	}
	questions := body["questions"].(map[string]any)
	route := questions["route"].(map[string]any)
	if route["type"] != "choice" {
		t.Fatalf("question = %#v", route)
	}
	criteria := route["criteria"].(map[string]any)
	if _, ok := criteria["billing"]; !ok {
		t.Fatalf("criteria = %#v", criteria)
	}
	if _, ok := criteria["technical"]; !ok {
		t.Fatalf("criteria = %#v", criteria)
	}
}

func TestScoredSwitchInvokeDisabledTie(t *testing.T) {
	poster := &scriptPoster{body: choicePayload(map[string]float64{"billing": 0.5, "technical": 0.5})}
	c := newTestSwitch(scoredSwitchParam{
		EscalateEnabled: boolPtr(false),
		EscalateTo:      []string{"node-esc"},
		MarginThreshold: floatPtr(0.9),
		MinTopScore:     floatPtr(0.99),
	}, poster)
	out, err := c.Invoke(t.Context(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if out["escalated"] != false || out["chosen_id"] != "opt-a" || out["chosen_label"] != "billing" || out["selected_handle"] != "opt-a" {
		t.Fatalf("out = %#v", out)
	}
	next, _ := out["_next"].([]any)
	if len(next) != 1 || next[0] != "node-a" {
		t.Fatalf("next = %#v", next)
	}
	if _, ok := out["scores"].(map[string]float64)["technical"]; !ok {
		t.Fatalf("scores = %#v", out["scores"])
	}
}

func TestScoredSwitchInvokeDefaultTieEscalates(t *testing.T) {
	poster := &scriptPoster{body: choicePayload(map[string]float64{"billing": 0.5, "technical": 0.5})}
	c := newTestSwitch(scoredSwitchParam{EscalateTo: []string{"node-esc"}}, poster)
	out, err := c.Invoke(t.Context(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if out["escalated"] != true || out["selected_handle"] != scoredSwitchEscalateHandle {
		t.Fatalf("out = %#v", out)
	}
}

func TestScoredSwitchInvokeMalformedAndRedactedError(t *testing.T) {
	poster := &scriptPoster{body: []byte(`{"answers":{}}`)}
	c := newTestSwitch(scoredSwitchParam{}, poster)
	if _, err := c.Invoke(t.Context(), nil, nil); err == nil || !strings.Contains(err.Error(), "unparseable") {
		t.Fatalf("err = %v", err)
	}
	poster = &scriptPoster{status: http.StatusUnauthorized, body: []byte("unauthorized")}
	c = newTestSwitch(scoredSwitchParam{APIKey: "secret-token"}, poster)
	_, err := c.Invoke(t.Context(), nil, nil)
	if err == nil || strings.Contains(err.Error(), "secret-token") || !strings.Contains(err.Error(), "401") {
		t.Fatalf("err = %v", err)
	}
}

func TestScoredSwitchInvokeMissingImageDoesNotCallModel(t *testing.T) {
	poster := &scriptPoster{body: choicePayload(map[string]float64{"billing": 1, "technical": 0})}
	state := runtime.NewCanvasState("run", "session")
	ctx := runtime.WithState(t.Context(), state)
	c := newTestSwitch(scoredSwitchParam{Image: "sys.files"}, poster)
	if _, err := c.Invoke(ctx, nil, nil); err == nil || !strings.Contains(err.Error(), "image") {
		t.Fatalf("err = %v", err)
	}
	if poster.calls != 0 {
		t.Fatalf("calls = %d", poster.calls)
	}
}

func tinyPNG(t *testing.T) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())
}

func TestScoredSwitchInvokeContentPartImage(t *testing.T) {
	poster := &scriptPoster{body: choicePayload(map[string]float64{"billing": 0.9, "technical": 0.1})}
	c := newTestSwitch(scoredSwitchParam{}, poster)
	out, err := c.Invoke(t.Context(), nil, map[string]any{"image": tinyPNG(t)})
	if err != nil {
		t.Fatal(err)
	}
	if out["chosen_label"] != "billing" {
		t.Fatalf("out = %#v", out)
	}
	var body map[string]any
	if err := json.Unmarshal(poster.got.Body, &body); err != nil {
		t.Fatal(err)
	}
	if _, ok := body["assets"]; ok {
		t.Fatal("content parts request must not send assets")
	}
	state, ok := body["state"].([]any)
	if !ok || len(state) != 2 {
		t.Fatalf("state = %#v", body["state"])
	}
	part := state[1].(map[string]any)
	if part["type"] != "image_url" {
		t.Fatalf("part = %#v", part)
	}
}

func TestScoredSwitchInvokeAssetImage(t *testing.T) {
	poster := &scriptPoster{body: choicePayload(map[string]float64{"billing": 0.9, "technical": 0.1})}
	c := newTestSwitch(scoredSwitchParam{BaseURL: "https://example.test/api/v1/systemone"}, poster)
	if _, err := c.Invoke(t.Context(), nil, map[string]any{"image": tinyPNG(t)}); err != nil {
		t.Fatal(err)
	}
	if poster.got.Endpoint != "https://example.test/api/v1/systemone" {
		t.Fatalf("endpoint = %s", poster.got.Endpoint)
	}
	if poster.got.Headers[imageOptInHeaderName] != "true" {
		t.Fatalf("headers = %#v", poster.got.Headers)
	}
	var body map[string]any
	if err := json.Unmarshal(poster.got.Body, &body); err != nil {
		t.Fatal(err)
	}
	assets, ok := body["assets"].(map[string]any)
	if !ok || len(assets) != 1 {
		t.Fatalf("assets = %#v", body["assets"])
	}
	state := body["state"].(map[string]any)
	if state["case"] == "" {
		t.Fatalf("state = %#v", state)
	}
}

func TestHTTPPosterSendsJSON(t *testing.T) {
	var gotAuth, gotType string
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotType = r.Header.Get("Content-Type")
		var err error
		gotBody, err = io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"answers":{"route":{"type":"choice","probabilities":{"billing":0.8,"technical":0.2}}}}`))
	}))
	defer srv.Close()
	body, status, err := httpScoringPoster{}.Post(t.Context(), scoringCall{
		Endpoint: srv.URL,
		APIKey:   "test-key",
		Body:     []byte(`{"model":"scoring-model"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if status != http.StatusOK || gotAuth != "Bearer test-key" || gotType != "application/json" {
		t.Fatalf("status=%d auth=%s type=%s", status, gotAuth, gotType)
	}
	if !bytes.Contains(gotBody, []byte("scoring-model")) || !bytes.Contains(body, []byte("probabilities")) {
		t.Fatalf("sent %s got %s", gotBody, body)
	}
}

func TestValidateScoredOptions(t *testing.T) {
	if err := validateScoredOptions(normalizeScoredOptions(sampleOptions())); err != nil {
		t.Fatal(err)
	}
	if err := validateScoredOptions(normalizeScoredOptions([]scoredOption{{Label: "only"}})); err == nil {
		t.Fatal("expected at least two options")
	}
	dupes := sampleOptions()
	dupes[1].Label = dupes[0].Label
	if err := validateScoredOptions(normalizeScoredOptions(dupes)); err == nil {
		t.Fatal("expected duplicate label error")
	}
}
