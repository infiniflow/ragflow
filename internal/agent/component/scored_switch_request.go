package component

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"net/url"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/image/webp"

	"ragflow/internal/agent/runtime"
)

const (
	requestFormatContentParts = "content_parts"
	requestFormatAssets       = "assets"
	// imageOptInHeaderName is the specimen (JEV 1.4) opt-in header for the
	// asset-reference endpoint. It is a wire string, not a product name.
	imageOptInHeaderName = "TypeSafe-Danger-Zone-Images"
	maxImageBytes        = 16 << 20
	maxImagesBytes       = 32 << 20
)

var errUnsupportedImage = errors.New("unsupported image")

type scoringImage struct {
	MIME    string
	Format  string
	DataURL string
	RawB64  string
	Bytes   []byte
}

type scoringCall struct {
	Endpoint string
	APIKey   string
	Headers  map[string]string
	Body     []byte
}

type scoringCallInput struct {
	Model        string
	BaseURL      string
	APIKey       string
	Format       string
	Instructions string
	Query        string
	Options      []scoredOption
	Images       []scoringImage
	Tasks        []scoringTask
}

func normalizeRequestFormat(raw string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", requestFormatContentParts:
		return requestFormatContentParts, nil
	case requestFormatAssets:
		return requestFormatAssets, nil
	default:
		return "", &ParamError{Field: "request_format", Reason: "must be content_parts or assets"}
	}
}

func scoringEndpoint(baseURL, format string) (string, string, error) {
	raw := strings.TrimSpace(baseURL)
	if raw == "" {
		return "", "", &ParamError{Field: "base_url", Reason: "the selected model has no API endpoint"}
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", "", &ParamError{Field: "base_url", Reason: "must be an absolute http(s) URL"}
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", "", &ParamError{Field: "base_url", Reason: "must be http or https"}
	}
	path := strings.TrimRight(parsed.Path, "/")
	wire := format
	switch {
	case hasSuffixFold(path, "/systemone"):
		wire = requestFormatAssets
	case hasSuffixFold(path, "/decisions"):
		wire = requestFormatContentParts
	default:
		path = stripSuffixFold(path, "/chat/completions")
		if format == requestFormatAssets {
			wire = requestFormatAssets
			path = strings.TrimRight(path, "/") + "/systemone"
		} else {
			wire = requestFormatContentParts
			path = decisionsPath(path)
		}
	}
	parsed.Path = path
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), wire, nil
}

func decisionsPath(path string) string {
	path = strings.TrimRight(path, "/")
	if rest, ok := trimSuffixFold(path, "/api/v1"); ok {
		return rest + "/api/alpha/decisions"
	}
	if rest, ok := trimSuffixFold(path, "/v1"); ok {
		return rest + "/alpha/decisions"
	}
	return path + "/decisions"
}

func hasSuffixFold(path, suffix string) bool {
	_, ok := trimSuffixFold(path, suffix)
	return ok
}

func stripSuffixFold(path, suffix string) string {
	if rest, ok := trimSuffixFold(path, suffix); ok {
		return rest
	}
	return path
}

func trimSuffixFold(path, suffix string) (string, bool) {
	if len(path) < len(suffix) || suffix == "" {
		return path, false
	}
	if strings.EqualFold(path[len(path)-len(suffix):], suffix) {
		return path[:len(path)-len(suffix)], true
	}
	return path, false
}

func buildScoringCall(in scoringCallInput) (scoringCall, error) {
	endpoint, wire, err := scoringEndpoint(in.BaseURL, in.Format)
	if err != nil {
		return scoringCall{}, err
	}
	if err := checkImageBudget(in.Images); err != nil {
		return scoringCall{}, err
	}
	instructions := strings.TrimSpace(in.Instructions)
	if instructions == "" {
		instructions = defaultScoringInstructions(wire, len(in.Images) > 0)
	}
	criteria := make(map[string]any, len(in.Options))
	for _, opt := range in.Options {
		if opt.Description == "" {
			criteria[opt.Label] = nil
			continue
		}
		criteria[opt.Label] = opt.Description
	}
	body := map[string]any{
		"model": in.Model,
		"questions": map[string]any{
			scoringQuestionID: map[string]any{
				"type":         "choice",
				"instructions": instructions,
				"criteria":     criteria,
			},
		},
	}
	questions := body["questions"].(map[string]any)
	for _, task := range in.Tasks {
		question, err := taskQuestion(task)
		if err != nil {
			return scoringCall{}, err
		}
		questions[task.ID] = question
	}
	var headers map[string]string
	needsImageHeader := len(in.Images) > 0 || tasksNeedImageHeader(in.Tasks)
	if len(in.Images) == 0 {
		body["state"] = in.Query
	} else if wire == requestFormatAssets {
		refs := make([]string, 0, len(in.Images))
		assets := make(map[string]any, len(in.Images))
		for i, img := range in.Images {
			ref := newImageRef(in.Query, i)
			refs = append(refs, ref)
			assets[ref] = map[string]any{
				"type":        "image",
				"format":      img.Format,
				"data_base64": img.RawB64,
			}
		}
		body["state"] = map[string]any{"case": in.Query, "images": refs}
		body["assets"] = assets
	} else {
		state := make([]any, 0, 1+len(in.Images))
		state = append(state, in.Query)
		for _, img := range in.Images {
			state = append(state, map[string]any{
				"type":      "image_url",
				"image_url": map[string]any{"url": img.DataURL},
			})
		}
		body["state"] = state
	}
	if needsImageHeader {
		headers = map[string]string{imageOptInHeaderName: "true"}
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return scoringCall{}, fmt.Errorf("encode scoring request: %w", err)
	}
	return scoringCall{
		Endpoint: endpoint,
		APIKey:   in.APIKey,
		Headers:  headers,
		Body:     encoded,
	}, nil
}

func defaultScoringInstructions(wire string, hasImages bool) string {
	if wire == requestFormatAssets && hasImages {
		return "Which option best matches the case in `case`?"
	}
	return "Which option best matches the case?"
}

func newImageRef(query string, index int) string {
	ref := fmt.Sprintf("img%d%s", index+1, strings.ReplaceAll(uuid.NewString(), "-", ""))
	if strings.Contains(query, ref) {
		ref += "x"
	}
	return ref
}

func checkImageBudget(images []scoringImage) error {
	total := 0
	for _, img := range images {
		if len(img.Bytes) > maxImageBytes {
			return &ParamError{Field: "image", Reason: "an image exceeds 16 MiB"}
		}
		total += len(img.Bytes)
	}
	if total > maxImagesBytes {
		return &ParamError{Field: "image", Reason: "images exceed 32 MiB combined"}
	}
	return nil
}

func gatherScoringImages(ctx context.Context, query, configured string, inputs map[string]any, format string) ([]scoringImage, error) {
	if inputs == nil {
		inputs = map[string]any{}
	}
	_, inInputs := inputs["image"]
	configured = strings.TrimSpace(configured)
	imageSet := inInputs || configured != ""
	var images []scoringImage
	if imageSet {
		var raw any
		if inInputs {
			raw = inputs["image"]
		} else {
			raw = resolveCanvasRef(ctx, configured)
		}
		found, err := imagesFromValue(raw, true, format)
		if err != nil {
			return nil, err
		}
		if len(found) == 0 {
			return nil, &ParamError{Field: "image", Reason: "image input is set but no image bytes were found"}
		}
		images = append(images, found...)
	}
	fromQuery, err := imagesFromString(query, false, format)
	if err != nil {
		return nil, err
	}
	images = append(images, fromQuery...)
	if !imageSet {
		if state, err := runtime.GetStateFromContext(ctx); err == nil && state != nil {
			_, uris := collectSysFiles(state)
			loose, err := imagesFromURIs(uris, false, format)
			if err != nil {
				return nil, err
			}
			images = append(images, loose...)
		}
	}
	images = dedupImages(images)
	if err := checkImageBudget(images); err != nil {
		return nil, err
	}
	return images, nil
}

func resolveScoredQuery(ctx context.Context, configured string, inputs map[string]any) (string, error) {
	if s, ok := stringFrom(inputs, "query"); ok && strings.TrimSpace(s) != "" {
		return strings.TrimSpace(s), nil
	}
	ref := strings.TrimSpace(configured)
	if ref == "" {
		ref = "sys.query"
	}
	if v := resolveCanvasRef(ctx, ref); v != nil {
		if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s), nil
		}
	}
	if !looksLikeVarRef(ref) {
		return ref, nil
	}
	return "", &ParamError{Field: "query", Reason: "required"}
}

func resolveCanvasRef(ctx context.Context, ref string) any {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil
	}
	if !looksLikeVarRef(ref) {
		return ref
	}
	state, err := runtime.GetStateFromContext(ctx)
	if err != nil || state == nil {
		return nil
	}
	v, err := state.GetVar(ref)
	if err != nil {
		return nil
	}
	return v
}

func looksLikeVarRef(ref string) bool {
	return strings.HasPrefix(ref, "sys.") || strings.HasPrefix(ref, "env.") || strings.Contains(ref, "@")
}

func imagesFromValue(v any, strict bool, format string) ([]scoringImage, error) {
	switch t := v.(type) {
	case nil:
		return nil, nil
	case string:
		return imagesFromString(t, strict, format)
	case []string:
		return imagesFromURIs(t, strict, format)
	case []any:
		var out []scoringImage
		for _, item := range t {
			part, err := imagesFromValue(item, strict, format)
			if err != nil {
				return nil, err
			}
			out = append(out, part...)
		}
		return out, nil
	case map[string]any:
		return imagesFromMap(t, strict, format)
	default:
		if strict {
			return nil, &ParamError{Field: "image", Reason: "image input is set but no image bytes were found"}
		}
		return nil, nil
	}
}

func imagesFromMap(m map[string]any, strict bool, format string) ([]scoringImage, error) {
	if nested, ok := m["image_url"].(map[string]any); ok {
		if s, ok := nested["url"].(string); ok {
			return imagesFromString(s, strict, format)
		}
	}
	for _, key := range []string{"url", "data_url", "data", "content", "image"} {
		if s, ok := m[key].(string); ok && strings.TrimSpace(s) != "" {
			return imagesFromString(s, strict, format)
		}
	}
	if b64, ok := m["data_base64"].(string); ok && strings.TrimSpace(b64) != "" {
		mime := "image/png"
		if s, ok := m["mime"].(string); ok && strings.TrimSpace(s) != "" {
			mime = s
		} else if s, ok := m["format"].(string); ok && strings.TrimSpace(s) != "" {
			mime = "image/" + strings.TrimPrefix(strings.ToLower(s), "image/")
		}
		return imagesFromString("data:"+mime+";base64,"+strings.TrimSpace(b64), strict, format)
	}
	if strict {
		return nil, &ParamError{Field: "image", Reason: "image input is set but no image bytes were found"}
	}
	return nil, nil
}

func imagesFromURIs(uris []string, strict bool, format string) ([]scoringImage, error) {
	var out []scoringImage
	for _, uri := range uris {
		part, err := imagesFromString(uri, strict, format)
		if err != nil {
			return nil, err
		}
		out = append(out, part...)
	}
	return out, nil
}

func imagesFromString(s string, strict bool, format string) ([]scoringImage, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	matches := dataImageRe.FindAllString(s, -1)
	if len(matches) == 0 {
		lower := strings.ToLower(s)
		if strict && (strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://")) {
			return nil, &ParamError{Field: "image", Reason: "remote image URLs are not accepted; provide image bytes"}
		}
		return nil, nil
	}
	var out []scoringImage
	for _, match := range matches {
		img, err := parseDataImage(match, format)
		if err != nil {
			if !strict && errors.Is(err, errUnsupportedImage) {
				continue
			}
			if errors.Is(err, errUnsupportedImage) {
				return nil, unsupportedImageError(format)
			}
			return nil, err
		}
		out = append(out, img)
	}
	return out, nil
}

func unsupportedImageError(format string) error {
	if format == requestFormatAssets {
		return &ParamError{Field: "image", Reason: "this request format accepts PNG or JPEG images"}
	}
	return &ParamError{Field: "image", Reason: "only PNG, JPEG, or WebP images are supported"}
}

func parseDataImage(uri, format string) (scoringImage, error) {
	const marker = ";base64,"
	lower := strings.ToLower(uri)
	idx := strings.Index(lower, marker)
	if !strings.HasPrefix(lower, "data:image/") || idx < 0 {
		return scoringImage{}, errUnsupportedImage
	}
	mime := strings.TrimPrefix(lower[:idx], "data:")
	if semi := strings.Index(mime, ";"); semi >= 0 {
		mime = mime[:semi]
	}
	wireFormat, ok := imageFormatForMIME(mime, format)
	if !ok {
		return scoringImage{}, errUnsupportedImage
	}
	payload := strings.TrimSpace(uri[idx+len(marker):])
	payload = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == ' ' || r == '\t' {
			return -1
		}
		return r
	}, payload)
	raw, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		raw, err = base64.URLEncoding.DecodeString(payload)
		if err != nil {
			return scoringImage{}, &ParamError{Field: "image", Reason: "image bytes could not be read"}
		}
	}
	if len(raw) == 0 {
		return scoringImage{}, &ParamError{Field: "image", Reason: "image input is set but no image bytes were found"}
	}
	if err := validateImageBytes(raw, wireFormat); err != nil {
		return scoringImage{}, err
	}
	encoded := base64.StdEncoding.EncodeToString(raw)
	mimeOut := mime
	if wireFormat == "jpg" {
		mimeOut = "image/jpeg"
	}
	return scoringImage{
		MIME:    mimeOut,
		Format:  wireFormat,
		DataURL: "data:" + mimeOut + ";base64," + encoded,
		RawB64:  encoded,
		Bytes:   raw,
	}, nil
}

func imageFormatForMIME(mime, requestFormat string) (string, bool) {
	switch mime {
	case "image/png":
		return "png", true
	case "image/jpeg", "image/jpg":
		return "jpg", true
	case "image/webp":
		if requestFormat == requestFormatAssets {
			return "", false
		}
		return "webp", true
	default:
		return "", false
	}
}

func validateImageBytes(raw []byte, format string) error {
	var err error
	switch format {
	case "webp":
		_, err = webp.DecodeConfig(bytes.NewReader(raw))
	default:
		_, _, err = image.DecodeConfig(bytes.NewReader(raw))
	}
	if err != nil {
		return &ParamError{Field: "image", Reason: "image bytes are not a readable image"}
	}
	return nil
}

func dedupImages(images []scoringImage) []scoringImage {
	seen := make(map[string]struct{}, len(images))
	out := make([]scoringImage, 0, len(images))
	for _, img := range images {
		if _, ok := seen[img.RawB64]; ok {
			continue
		}
		seen[img.RawB64] = struct{}{}
		out = append(out, img)
	}
	return out
}
