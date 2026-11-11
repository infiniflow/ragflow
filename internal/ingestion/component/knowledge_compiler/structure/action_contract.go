package structure

import (
	"fmt"
	"log"
	"strings"

	"ragflow/internal/ingestion/component/knowledge_compiler/common"
	"ragflow/internal/ontology/contract"
)

// Action contract assembly: from the extractor's JSON payload to the two columns
// the row carries.
//
// The design names one new output field, `contract_quotes` (the verbatim sentence
// per contract field). A value needs a companion: `contract` carries the six
// values, `contract_quotes` carries where each one was read. Both are requested
// only when the template declares an `action` entity, so a model that has no
// actions pays nothing for this (see withActionContractFields).
//
// Everything the extraction produces is a proposal. Whether a field ends up
// `present` is decided by contract.Status; whether the value survives is decided
// by the evidence gate: a value with no verbatim quote is a hallucination and is
// dropped, which is also what keeps the D15 correction meaningful (it needs a
// quote to locate the sentence).

// actionContractFieldNames mirrors the fixed six, for prompt rendering.
var actionContractFieldNames = contract.ActionContractFields()

// withActionContractFields appends the two contract output fields to the entity
// stage when (and only when) the template declares an action entity.
//
// The prompt is the only place the model learns that these keys exist: a key
// described inside a entity's `rule` prose but missing from the response skeleton
// is silently dropped (see renderTypeFields), so the skeleton must carry them.
func withActionContractFields(extra []any, entityFields []any) []any {
	declared, isAction := actionEntityContractFields(entityFields)
	if !isAction {
		return extra
	}
	out := append([]any{}, extra...)
	seen := map[string]bool{}
	for _, raw := range out {
		if field, ok := raw.(map[string]any); ok {
			if name, _ := field["name"].(string); name != "" {
				seen[name] = true
			}
		}
	}
	if !seen["contract"] {
		out = append(out, actionContractValueField(declared))
	}
	if !seen["contract_quotes"] {
		out = append(out, actionContractQuoteField(declared))
	}
	return out
}

// actionEntityContractFields returns which contract fields this model asks for,
// and whether it declares an action entity at all.
//
// A template can narrow the contract: a model whose actions are pure lookups
// should not be paying prompt tokens for `failure_policy`, and a model that
// declares nothing gets all six — the safe reading, because "this action needs
// no fields" is the one interpretation that lets something run unchecked.
//
// The declaration lives on the entity as a "|"-separated string (the same shape
// `aliases` uses); the service accepts a list too, because a template written by
// hand may carry either.
func actionEntityContractFields(entityFields []any) ([]string, bool) {
	for _, raw := range entityFields {
		field, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		typ, _ := field["type"].(string)
		if !strings.EqualFold(strings.TrimSpace(typ), contract.ActionEntityType) {
			continue
		}
		return declaredContractFields(field["contract"]), true
	}
	return nil, false
}

// declaredContractFields normalises a entity's declaration against the fixed six.
//
// The normalisation itself lives in contract.NormalizeDeclared, because the same
// list is what the stored contract is later judged against: two normalisations
// would let a template ask for four fields and be judged against five.
func declaredContractFields(raw any) []string {
	var candidates []string
	switch value := raw.(type) {
	case string:
		candidates = strings.Split(value, "|")
	case []any:
		for _, item := range value {
			if text, ok := item.(string); ok {
				candidates = append(candidates, text)
			}
		}
	case []string:
		candidates = append(candidates, value...)
	}
	return contract.NormalizeDeclared(candidates)
}

// actionDeclaredFields returns the contract fields a template's action entity asks
// for, or nil when the template declares no action entity.
func actionDeclaredFields(parserConfig map[string]any) []string {
	declared, isAction := actionEntityContractFields(configFields(common.GetMap(parserConfig, "entity")))
	if !isAction {
		return nil
	}
	return declared
}

func actionContractValueField(declared []string) map[string]any {
	return map[string]any{
		"name":     "contract",
		"type":     "object",
		"required": false,
		"description": "Execution contract of an action. Fill only what the source states; " +
			"omit a field the source does not state (do not write \"no restriction\" for it). " +
			"authorization is the role PERMITTED to approve the action; the person who carries " +
			"the step out is NOT the authorization. audit_required is a boolean.",
		"shape": contractShape(declared),
	}
}

func actionContractQuoteField(declared []string) map[string]any {
	return map[string]any{
		"name":     "contract_quotes",
		"type":     "object",
		"required": false,
		"description": "For every contract field you filled, the sentence you read it from, " +
			"copied character for character from the source. A paraphrase cannot be verified " +
			"and will be discarded together with its value. Use the same field names as contract.",
		"shape": quoteShape(declared),
	}
}

// contractShape renders the response skeleton for the declared fields only.
//
// The skeleton is what the model copies: a key absent here is a key it does not
// emit, which is the mechanism that makes a narrow declaration actually narrow.
func contractShape(declared []string) string {
	parts := make([]string, 0, len(declared))
	for _, name := range declared {
		switch name {
		case "atomic_effects", "follow_ups":
			parts = append(parts, fmt.Sprintf("%q: [\"<string>\"]", name))
		case "audit_required":
			parts = append(parts, `"audit_required": "<true|false>"`)
		default:
			parts = append(parts, fmt.Sprintf("%q: \"<string>\"", name))
		}
	}
	return "{" + strings.Join(parts, ", ") + "}"
}

// quoteShape is the same skeleton with a verbatim sentence per field.
func quoteShape(declared []string) string {
	parts := make([]string, 0, len(declared))
	for _, name := range declared {
		parts = append(parts, fmt.Sprintf("%q: \"<verbatim sentence>\"", name))
	}
	return "{" + strings.Join(parts, ", ") + "}"
}

// ActionContractFromPayload builds the contract an entity payload proposes.
//
// It accepts the three shapes producers actually emit, because the extractor's
// output is not schema-validated: an explicit `contract` object, the six keys at
// the top level, and the `contract_quotes` companion in either place. The second
// shape matters for templates that also declare the fields as attributes.
func ActionContractFromPayload(payload map[string]any) (contract.ActionContract, bool) {
	out := contract.NewActionContract()
	if len(payload) == 0 {
		return out, false
	}
	quotes := contractQuoteMap(payload["contract_quotes"])
	found := false
	if raw, ok := payload["contract"]; ok {
		if entries, ok := raw.(map[string]any); ok {
			found = true
			for name, value := range entries {
				if entry, ok := value.(map[string]any); ok {
					quote, _ := entry["q"].(string)
					out.Set(name, entry["v"], quote)
					continue
				}
				out.Set(name, value, quotes[name])
			}
		} else if typed, ok := raw.(contract.ActionContract); ok {
			out = typed.Clone()
			found = true
		}
	}
	for _, name := range actionContractFieldNames {
		if out.Status(name) != contract.ContractStatusMissing {
			continue
		}
		value, ok := payload[name]
		if !ok {
			continue
		}
		out.Set(name, value, quotes[name])
		found = true
	}
	// A value that came from the top level still deserves its quote.
	for name, quote := range quotes {
		if strings.TrimSpace(quote) == "" || out.Status(name) == contract.ContractStatusMissing {
			continue
		}
		if out.Quote(name) == "" {
			out.Set(name, out.Value(name), quote)
		}
	}
	return out, found && !out.IsEmpty()
}

func contractQuoteMap(raw any) map[string]string {
	out := map[string]string{}
	switch v := raw.(type) {
	case map[string]any:
		for name, value := range v {
			if quote, ok := value.(string); ok && strings.TrimSpace(quote) != "" {
				out[name] = strings.TrimSpace(quote)
			}
		}
	case map[string]string:
		for name, quote := range v {
			if strings.TrimSpace(quote) != "" {
				out[name] = strings.TrimSpace(quote)
			}
		}
	}
	return out
}

// ClearPerformerAuthorizations applies D15 to gated payloads.
//
// It runs after the evidence gate on purpose: the correction needs the field's
// quote, and the gate is what proves the quote is really in the source. A payload
// whose authorization names the executor is cleared here, before the row is
// built, so no downstream view can ever read it as a permission.
//
// Returns how many values were cleared, for the quality log line.
func ClearPerformerAuthorizations(payloads []map[string]any, textByID map[string]string) int {
	cleared := 0
	for _, payload := range payloads {
		action, ok := ActionContractFromPayload(payload)
		if !ok {
			continue
		}
		quote := action.Quote(contract.AuthorizationField)
		if quote == "" {
			continue
		}
		corrected, changed := contract.ClearPerformerAsAuthorization(action, contractSourceText(payload, textByID, quote))
		if !changed {
			continue
		}
		payload["contract"] = corrected
		// The value is gone, so its quote must go too: a leftover quote would be
		// re-attached to the next value the extractor writes.
		if quotes, ok := payload["contract_quotes"].(map[string]any); ok {
			delete(quotes, contract.AuthorizationField)
		}
		log.Printf("[d15] cleared action=%s field=%s reason=no_permission_wording",
			firstStringOf(payload["name"]), contract.AuthorizationField)
		cleared++
	}
	return cleared
}

// contractSourceText finds the chunk text the quote came from: the field's quote
// may point at any of the payload's source chunks, and only the containing one can
// answer "does this sentence talk about permission".
func contractSourceText(payload map[string]any, textByID map[string]string, quote string) string {
	if len(textByID) == 0 {
		return quote
	}
	for _, id := range payloadChunkIDs(payload, nil) {
		text := textByID[id]
		if text == "" {
			continue
		}
		if strings.Contains(text, quote) {
			return text
		}
	}
	// The quote may belong to a chunk the payload cited only implicitly; scanning
	// the batch is bounded by the batch size and keeps the correction working.
	for _, text := range textByID {
		if strings.Contains(text, quote) {
			return text
		}
	}
	return quote
}
