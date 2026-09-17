package component

import "strings"

// splitCompositeLLMID extracts the provider driver and bare model id from the
// canvas llm_id convention:
//   - "model@provider"          -> ("model", "provider", true)
//   - "model@instance@provider" -> ("model", "provider", true)
//   - "model@quant@inst@prov"   -> ("model@quant", "prov", true)
//   - bare "model"              -> ("model", "", false)
//
// The split is right-anchored on the provider (last '@' segment) so model names
// that legitimately contain '@' (e.g. LM Studio quant suffixes) are preserved.
// Matches Python's split_model_name (rsplit("@", 2)) and service-layer
// parseModelName / BaseModelName.
func splitCompositeLLMID(s string) (modelName, driver string, hasDriver bool) {
	s = strings.TrimSpace(s)
	parts := strings.Split(s, "@")
	n := len(parts)
	switch {
	case n < 2:
		return s, "", false
	case n == 2:
		return parts[0], parts[1], true
	default:
		// n >= 3: provider is last; model is everything before instance+provider.
		return strings.Join(parts[:n-2], "@"), parts[n-1], true
	}
}
