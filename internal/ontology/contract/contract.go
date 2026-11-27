// Package contract is the R3 action-contract layer: the six fixed fields an
// action must state, their three-state judgement, and the execution verdict.
//
// It lives in its own leaf package because both the compiler
// (internal/ingestion/...) and the service/handler/agent layers judge contracts,
// while the compiler cannot import internal/service. A pure, dependency-free
// layer is the only shape that keeps one definition of the rule instead of two
// that drift.
package contract

import (
	"encoding/json"
	"regexp"
	"strings"
	"unicode"
)

// Action contract: the sixth rule layer of the ontology.
//
// RAGFlow already carries three kinds of "rule" and they are judged by different
// parties, so they must not be merged:
//
//	R1 extraction rule  field.rule                     judged by the LLM, at compile time
//	R2 structure constraint parent/domain/range/datatype judged by code, at compile/publish time
//	R3 action contract  field.contract.*                judged by a human, at execution time
//
// This file owns R3 only. It is a pure-function layer: it never touches the
// engine, the database or a template, so the same code runs in the writer, in the
// review queue and in the agent tool.
//
// The contract lives in a single json column (`contract`) plus a projection of it
// (`contract_missing_kwd`) that exists only so "which actions lack authorization"
// is one query instead of a full scan. Everything else — provenance, what a
// recompile may overwrite — is derived from the presence of the field's quote:
//
//	value + quote = extracted from the document  -> a recompile may overwrite it
//	value only    = typed in by an operator      -> a recompile must keep it
//	no value      = nobody said anything         -> "the document does not specify it"
//
// That single rule replaces origin_kwd / manual_fields_kwd / provenance columns,
// and its only obligation is that an operator edit deletes the quote.

// ActionContractVersion is the structure version written as `_v`. It is not a
// field: it exists so a future shape (structured authorization, translations,
// several contracts per action) can be read by branching on it, without adding a
// column or migrating data.
const ActionContractVersion = 1

// ActionEntityType is the ontology entity an action is declared under. An action is
// an ordinary ABox instance that additionally carries a contract — not a separate
// layer, so the review queue, the version stamp and the retrieval filters all
// apply to it unchanged.
const ActionEntityType = "action"

// AuthorizationField is the only field that can be explicitly absent from the
// document ("no approval required"). It decides whether an action may ever be
// executed automatically, so it is treated differently everywhere.
const AuthorizationField = "authorization"

// actionContractFields is the fixed set of contract fields, in display order.
//
// The admission rule is "without it you cannot decide whether the action may
// run", not "the document might mention it". That is why parameters (inputs) and
// definition are absent, and why valid_until / idempotent / rate_limit are absent:
// they either make every action look incomplete or they belong to the binding.
var actionContractFields = []string{
	AuthorizationField,
	"preconditions",
	"atomic_effects",
	"follow_ups",
	"failure_policy",
	"audit_required",
}

// ActionContractFields returns the fixed contract field names in display order.
func ActionContractFields() []string {
	out := make([]string, len(actionContractFields))
	copy(out, actionContractFields)
	return out
}

// NormalizeDeclared turns a template's declaration into the fields a contract is
// judged against.
//
// The template may narrow the contract — a model whose actions are pure lookups
// does not need `failure_policy` asked of the model that extracts them — and the
// declaration therefore decides two things at once: which fields the extractor is
// asked for, and which fields a reader may call missing. Keeping both here is what
// stops the prompt and the judgement from disagreeing about what is owed.
//
// Two rules:
//   - a name that is not a contract field is dropped (the template is validated
//     before it is saved, so this only guards rows compiled by an older one);
//   - authorization is always required. It decides whether the action may ever
//     run unattended, and a field nobody was asked for can never be stated — so
//     narrowing it away would pin every action of that model at `manual` forever
//     instead of letting the document say "no approval required".
//
// Nothing recognised means all six: "this action needs no fields" is the one
// reading that would let something run unchecked.
func NormalizeDeclared(names []string) []string {
	allowed := map[string]bool{}
	for _, name := range actionContractFields {
		allowed[name] = true
	}
	recognised := make([]string, 0, len(names))
	seen := map[string]bool{}
	for _, name := range names {
		name = strings.TrimSpace(name)
		if !allowed[name] || seen[name] {
			continue
		}
		seen[name] = true
		recognised = append(recognised, name)
	}
	if len(recognised) == 0 {
		return ActionContractFields()
	}
	if seen[AuthorizationField] {
		return recognised
	}
	return append([]string{AuthorizationField}, recognised...)
}

// ContractFieldStatus is the three-state judgement of one contract field.
type ContractFieldStatus string

const (
	// ContractStatusPresent means the document states this field.
	ContractStatusPresent ContractFieldStatus = "present"
	// ContractStatusMissing means the document does not state it. It does NOT mean
	// "unrestricted": a missing authorization is unknown, not absent.
	ContractStatusMissing ContractFieldStatus = "missing"
	// ContractStatusExplicitNone means the document says no requirement applies.
	ContractStatusExplicitNone ContractFieldStatus = "explicit_none"
)

// ContractProvenance answers "where did this value come from", which the UI and
// the agent must not confuse: a manual value may not be cited as document
// evidence.
type ContractProvenance string

const (
	ContractProvenanceDocument ContractProvenance = "document"
	ContractProvenanceManual   ContractProvenance = "manual"
	ContractProvenanceAbsent   ContractProvenance = "absent"
)

// ContractVerdict is the execution judgement, ordered by strictness
// refuse > manual > confirm > allow.
type ContractVerdict string

const (
	ContractVerdictRefuse  ContractVerdict = "refuse"
	ContractVerdictManual  ContractVerdict = "manual"
	ContractVerdictConfirm ContractVerdict = "confirm"
	ContractVerdictAllow   ContractVerdict = "allow"
)

// Verdict reason codes. They are stable strings because the UI renders them and
// the audit log stores them.
const (
	VerdictReasonActionNotFound       = "action_not_found"
	VerdictReasonTemplateUnpublished  = "template_not_published"
	VerdictReasonToolUnavailable      = "tool_unavailable"
	VerdictReasonUnbound              = "unbound"
	VerdictReasonBindingDisabled      = "binding_disabled"
	VerdictReasonNotQACallable        = "not_qa_callable"
	VerdictReasonAuthorizationMissing = "authorization_missing"
	VerdictReasonAuthorizationPresent = "authorization_present"
	VerdictReasonPreconditionsPresent = "preconditions_present"
	VerdictReasonNotReadOnly          = "not_read_only"
	VerdictReasonExecDisabled         = "exec_disabled"
	VerdictReasonAllowed              = "allowed"
)

// contractStrictness orders verdicts so several reasons collapse into the most
// restrictive one.
var contractStrictness = map[ContractVerdict]int{
	ContractVerdictAllow:   0,
	ContractVerdictConfirm: 1,
	ContractVerdictManual:  2,
	ContractVerdictRefuse:  3,
}

// ActionContractField is one field's value and its verbatim quote.
//
// Q is deliberately the source of truth for provenance: an extracted value always
// carries the sentence it came from (the evidence gate drops values that do not
// quote the source), so "no quote" can only mean "an operator typed it".
type ActionContractField struct {
	V any    `json:"v,omitempty"`
	Q string `json:"q,omitempty"`
}

// ActionContract is the parsed `contract` column.
//
// The zero value is usable: every lookup reports "missing", which is exactly how a
// row compiled before this column existed must be read.
type ActionContract struct {
	fields map[string]ActionContractField
	// required is the declaration this contract was extracted under (the action
	// entity's `contract` narrowing, see NormalizeDeclared). It travels with the
	// contract — serialised as `_d` — because everything that judges a row later
	// (the review queue, the action workbench, the agent tool) reads the row, not
	// the template, and would otherwise call a field the extractor was never asked
	// for "missing". Empty means all six, which is what rows compiled before the
	// declaration existed mean.
	required []string
	// malformed records that the column could not be parsed. Callers surface it as
	// "missing" but the receipt keeps the fact so a broken row is visible instead of
	// silently pretending the document said nothing.
	malformed bool
}

// NewActionContract returns an empty contract.
func NewActionContract() ActionContract {
	return ActionContract{fields: map[string]ActionContractField{}}
}

// ParseActionContract reads the `contract` column.
//
// An empty or unparseable column yields the empty contract, never an error: a row
// without a contract means "nobody extracted one", which the verdict turns into
// `manual` (never `allow`), so the failure mode is safe.
func ParseActionContract(raw any) ActionContract {
	out := NewActionContract()
	switch v := raw.(type) {
	case nil:
		return out
	case ActionContract:
		return v.Clone()
	case map[string]ActionContractField:
		for k, f := range v {
			out.fields[k] = f
		}
		return out
	case map[string]any:
		out.assignRaw(v)
		return out
	case string:
		return parseActionContractJSON([]byte(v))
	case []byte:
		return parseActionContractJSON(v)
	case json.RawMessage:
		return parseActionContractJSON(v)
	default:
		// Engine columns can come back marshalled through an intermediate
		// representation; re-marshalling is cheaper than a type switch per engine.
		if b, err := json.Marshal(v); err == nil {
			return parseActionContractJSON(b)
		}
		out.malformed = true
		return out
	}
}

func parseActionContractJSON(raw []byte) ActionContract {
	out := NewActionContract()
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return out
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(trimmed), &decoded); err != nil {
		out.malformed = true
		return out
	}
	out.assignRaw(decoded)
	return out
}

func (c *ActionContract) assignRaw(raw map[string]any) {
	if c.fields == nil {
		c.fields = map[string]ActionContractField{}
	}
	for name, value := range raw {
		switch name {
		case "_v":
			continue
		case "_d":
			if declared := declaredList(value); len(declared) > 0 {
				c.required = NormalizeDeclared(declared)
			}
			continue
		}
		switch v := value.(type) {
		case nil:
			continue
		case map[string]any:
			field := ActionContractField{}
			if val, ok := v["v"]; ok {
				field.V = val
			}
			if q, ok := v["q"].(string); ok {
				field.Q = strings.TrimSpace(q)
			}
			if contractValueEmpty(field.V) && field.Q == "" {
				continue
			}
			c.fields[name] = field
		default:
			// A bare scalar is accepted: hand-written rows and older payloads store
			// the value directly. It has no quote, so it reads as manual.
			if contractValueEmpty(v) {
				continue
			}
			c.fields[name] = ActionContractField{V: v}
		}
	}
}

// Malformed reports whether the column could not be parsed.
func (c ActionContract) Malformed() bool { return c.malformed }

// Clone returns a deep-enough copy: values are scalars or string slices, which
// Set replaces instead of mutating, so sharing them is safe.
func (c ActionContract) Clone() ActionContract {
	out := NewActionContract()
	out.malformed = c.malformed
	if len(c.required) > 0 {
		out.required = append([]string{}, c.required...)
	}
	for k, v := range c.fields {
		out.fields[k] = v
	}
	return out
}

// SetDeclared records the declaration this contract is judged against.
//
// It is written by the compiler from the action entity's `contract` narrowing, so
// every later reader can judge the row without loading the template.
func (c *ActionContract) SetDeclared(names []string) {
	c.required = NormalizeDeclared(names)
}

// RequiredFields lists the fields this contract is judged against, in display
// order. An undeclared contract is judged against all six.
func (c ActionContract) RequiredFields() []string {
	out := make([]string, len(c.requiredOrAll()))
	copy(out, c.requiredOrAll())
	return out
}

// requiredOrAll is the internal form: it avoids a copy on the hot judging paths
// and returns the package slice, which callers must not modify.
func (c ActionContract) requiredOrAll() []string {
	if len(c.required) == 0 {
		return actionContractFields
	}
	return c.required
}

// Field returns one field's value and quote.
func (c ActionContract) Field(name string) (ActionContractField, bool) {
	if c.fields == nil {
		return ActionContractField{}, false
	}
	f, ok := c.fields[name]
	if !ok {
		return ActionContractField{}, false
	}
	return f, true
}

// Value returns the field's value, or nil.
func (c ActionContract) Value(name string) any {
	f, ok := c.Field(name)
	if !ok || contractValueEmpty(f.V) {
		return nil
	}
	return f.V
}

// Quote returns the field's verbatim quote, or "".
func (c ActionContract) Quote(name string) string {
	f, _ := c.Field(name)
	return f.Q
}

// Status judges one field.
//
// explicit_none is only reachable for authorization, and only from the value
// itself: the extractor must have written a "no approval required" statement.
// Missing stays missing — never re-read as "unrestricted".
func (c ActionContract) Status(name string) ContractFieldStatus {
	f, ok := c.Field(name)
	if !ok || contractValueEmpty(f.V) {
		return ContractStatusMissing
	}
	if name == AuthorizationField {
		if text, ok := f.V.(string); ok && statesNoAuthorization(text) {
			return ContractStatusExplicitNone
		}
	}
	return ContractStatusPresent
}

// Provenance derives where the value came from.
func (c ActionContract) Provenance(name string) ContractProvenance {
	f, ok := c.Field(name)
	if !ok || contractValueEmpty(f.V) {
		return ContractProvenanceAbsent
	}
	if strings.TrimSpace(f.Q) == "" {
		return ContractProvenanceManual
	}
	return ContractProvenanceDocument
}

// MissingFields lists the required fields without a value, in display order.
//
// Only required fields can be missing: a field the template never asked for is
// not owed, so calling it missing would show every action of a narrow model as
// permanently incomplete.
func (c ActionContract) MissingFields() []string {
	required := c.requiredOrAll()
	out := make([]string, 0, len(required))
	for _, name := range required {
		if c.Status(name) == ContractStatusMissing {
			out = append(out, name)
		}
	}
	return out
}

// DeclaredCount counts the required fields that have a value (any provenance).
func (c ActionContract) DeclaredCount() int {
	n := 0
	for _, name := range c.requiredOrAll() {
		if c.Status(name) != ContractStatusMissing {
			n++
		}
	}
	return n
}

// IsEmpty reports a contract with no usable field. Rows written before the column
// existed read as empty, and an empty contract must be treated as "all missing"
// rather than trusting contract_missing_kwd, which is empty on those rows too.
func (c ActionContract) IsEmpty() bool {
	for _, name := range actionContractFields {
		if c.Status(name) != ContractStatusMissing {
			return false
		}
	}
	return true
}

// Set writes one field's value and quote.
func (c *ActionContract) Set(name string, value any, quote string) {
	if c.fields == nil {
		c.fields = map[string]ActionContractField{}
	}
	if contractValueEmpty(value) {
		delete(c.fields, name)
		return
	}
	c.fields[name] = ActionContractField{V: value, Q: strings.TrimSpace(quote)}
}

// SetManual writes an operator-maintained value.
//
// Deleting the quote is the whole mechanism: it flips provenance to manual, which
// both protects the value from the next compile and forbids citing a chunk for it.
func (c *ActionContract) SetManual(name string, value any) {
	c.Set(name, value, "")
}

// MergeExtracted folds a freshly extracted contract into an existing one.
//
// The only rule: a field is overwritten when the new extraction carries a quote.
// No quote means either "not extracted this time" or "the old value is manual" —
// both must keep the old value. That is why a recompile can never clobber an
// operator's edit, without any extra bookkeeping column.
func (c ActionContract) MergeExtracted(next ActionContract) ActionContract {
	out := c.Clone()
	// The declaration comes from the template, not from the document, so a
	// recompile is the moment a re-widened (or re-narrowed) declaration takes
	// effect on an existing row. An extraction that carries none is a compile that
	// predates the declaration, and must not clear the one already recorded.
	if len(next.required) > 0 {
		out.required = append([]string{}, next.required...)
	}
	for name, field := range next.fields {
		if contractValueEmpty(field.V) {
			continue
		}
		if strings.TrimSpace(field.Q) == "" {
			continue
		}
		out.fields[name] = field
	}
	return out
}

// MarshalJSON writes the flat shape the column stores: `_v` alongside the fields,
// and `_d` for the declaration when it narrows the six.
func (c ActionContract) MarshalJSON() ([]byte, error) {
	out := make(map[string]any, len(c.fields)+2)
	out["_v"] = ActionContractVersion
	if len(c.required) > 0 {
		out["_d"] = append([]string{}, c.required...)
	}
	for name, field := range c.fields {
		if contractValueEmpty(field.V) {
			continue
		}
		entry := map[string]any{"v": field.V}
		if field.Q != "" {
			entry["q"] = field.Q
		}
		out[name] = entry
	}
	return json.Marshal(out)
}

// UnmarshalJSON accepts the flat column shape.
func (c *ActionContract) UnmarshalJSON(raw []byte) error {
	parsed := parseActionContractJSON(raw)
	*c = parsed
	return nil
}

// ActionContractFieldView is one field as the API and the UI need it.
type ActionContractFieldView struct {
	Status     ContractFieldStatus `json:"status"`
	Value      any                 `json:"value,omitempty"`
	Quote      string              `json:"quote,omitempty"`
	Provenance ContractProvenance  `json:"provenance"`
}

// ActionContractView is the contract as a set of judged fields.
type ActionContractView struct {
	Fields  map[string]ActionContractFieldView `json:"fields"`
	Missing []string                           `json:"missing"`
	// Required is the declaration this contract is judged against. It is the
	// denominator of "Declared": the UI must not say 2/6 about a contract that was
	// only ever asked for two fields.
	Required []string `json:"required"`
	// Declared is "4/6" in the UI: how many required fields the document or an
	// operator filled.
	Declared int `json:"declared"`
}

// View projects the contract for transport.
//
// Fields still carries all six: a value stored before a template narrowed its
// declaration is real, and dropping it from the view would hide it from the
// operator who has to edit it. Required says which of them were asked for.
func (c ActionContract) View() ActionContractView {
	view := ActionContractView{
		Fields:   make(map[string]ActionContractFieldView, len(actionContractFields)),
		Missing:  c.MissingFields(),
		Required: c.RequiredFields(),
	}
	for _, name := range actionContractFields {
		field, _ := c.Field(name)
		view.Fields[name] = ActionContractFieldView{
			Status:     c.Status(name),
			Value:      c.Value(name),
			Quote:      field.Q,
			Provenance: c.Provenance(name),
		}
	}
	view.Declared = c.DeclaredCount()
	return view
}

// contractValueEmpty is the single definition of "this field has no value".
//
// A boolean false is a value ("no audit needed"), an empty string and an empty
// list are not. Keeping this in one place is what makes the missing column and the
// status computation impossible to drift apart.
func contractValueEmpty(value any) bool {
	switch v := value.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(v) == ""
	case []any:
		return len(v) == 0
	case []string:
		if len(v) == 0 {
			return true
		}
		for _, item := range v {
			if strings.TrimSpace(item) != "" {
				return false
			}
		}
		return true
	case map[string]any:
		return len(v) == 0
	}
	return false
}

// ContractMissingFields is a convenience for the writer: it computes the parallel
// column from the contract in one call.
func ContractMissingFields(contract ActionContract) []string {
	return contract.MissingFields()
}

// declaredList reads the `_d` column entry: the list this package writes, the
// `[]any` a JSON decode produces, or the template's own "|"-separated string.
func declaredList(raw any) []string {
	switch v := raw.(type) {
	case []string:
		return v
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if text, ok := item.(string); ok {
				out = append(out, text)
			}
		}
		return out
	case string:
		return strings.Split(v, "|")
	}
	return nil
}

// ---------------------------------------------------------------------------
// Language-branching text judgements
// ---------------------------------------------------------------------------

// The three states of authorization and the D15 correction are text judgements,
// so they need the document's language. The design scopes them to zh and en;
// anything else only gets present/missing, because a wrong "explicit none" reads
// as "no approval required" and would let a dangerous action run unattended.
//
// A Chinese regular expression ported verbatim to English documents silently never
// matches, so both branches exist and are chosen by the quote's own text rather
// than by a tenant setting: one corpus can mix languages.

var (
	explicitNoneZH = regexp.MustCompile(`无需(授权|审批|权限|审核)|无权限要求|不限(人员|权限)|任何人(均|都)可`)
	explicitNoneEN = regexp.MustCompile(`(?i)no\s+(authorization|authorisation|approval|permission|review)\s+(is\s+)?(required|needed)|anyone\s+(may|can)|no\s+restrictions?`)

	// D15: the wording that marks a sentence as being about *who is permitted*,
	// as opposed to *who carries out* the step.
	permissionWordingZH = regexp.MustCompile(`授权|审批|权限|有权|须经|同意|核准|签批|批准`)
	permissionWordingEN = regexp.MustCompile(`(?i)authoriz|authoris|approv|permission|entitled|may only|sign-?off|requires?\s+(approval|signature)`)
)

// ContractTextLang classifies the language a judgement should use: "zh", "en" or
// "other". It looks at the text being judged, because a single corpus routinely
// mixes languages and a tenant-level switch would misjudge the minority.
//
// Latin script alone does not make a sentence English. Treating German as English
// would run the D15 correction with patterns that cannot match its wording and
// delete correct values, so English is confirmed by function words and vetoed by
// a competing Latin language; everything else stays "other" and is never cleared.
func ContractTextLang(text string) string {
	hasLatin := false
	for _, r := range text {
		if unicode.Is(unicode.Han, r) {
			return "zh"
		}
		if r < unicode.MaxASCII && unicode.IsLetter(r) {
			hasLatin = true
		}
	}
	if !hasLatin {
		return "other"
	}

	englishHits, foreignHits := 0, 0
	for _, word := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r)
	}) {
		switch {
		case englishFunctionWords[word]:
			englishHits++
		case nonEnglishMarkers[word]:
			foreignHits++
		}
	}
	if foreignHits > 0 && foreignHits >= englishHits {
		return "other"
	}
	if englishHits > 0 {
		return "en"
	}
	return "other"
}

// englishFunctionWords are frequent, language-specific English tokens. Domain
// words (approval, permission) are included because an authorization sentence is
// short and often carries only one function word.
var englishFunctionWords = map[string]bool{
	"the": true, "a": true, "an": true, "is": true, "are": true, "was": true,
	"were": true, "be": true, "must": true, "shall": true, "should": true,
	"may": true, "can": true, "of": true, "to": true, "and": true, "or": true,
	"for": true, "by": true, "with": true, "from": true, "this": true,
	"that": true, "it": true, "no": true, "not": true, "any": true, "only": true,
	"required": true, "required.": true, "approval": true, "permission": true,
	"authorized": true, "authorised": true, "entitled": true, "restrictions": true,
}

// nonEnglishMarkers are high-frequency function words of other Latin-script
// languages RAGFlow corpora actually contain. They only need to outnumber the
// English hits, so ambiguity is harmless.
var nonEnglishMarkers = map[string]bool{
	// German
	"der": true, "die": true, "das": true, "und": true, "oder": true, "ist": true,
	"sind": true, "wird": true, "werden": true, "von": true, "vom": true, "mit": true,
	"für": true, "nicht": true, "kein": true, "keine": true, "durch": true,
	// French
	"le": true, "la": true, "les": true, "des": true, "du": true, "et": true,
	"ou": true, "est": true, "une": true, "aucune": true, "par": true,
	// Spanish / Portuguese
	"el": true, "los": true, "las": true, "es": true, "de": true, "da": true,
	"do": true, "não": true, "não.": true, "para": true, "por": true,
}

// statesNoAuthorization reports an explicit "no approval required" statement.
func statesNoAuthorization(text string) bool {
	switch ContractTextLang(text) {
	case "zh":
		return explicitNoneZH.MatchString(text)
	case "en":
		return explicitNoneEN.MatchString(text)
	}
	return false
}

// hasPermissionWording reports whether a sentence talks about authorization.
func hasPermissionWording(text string) bool {
	switch ContractTextLang(text) {
	case "zh":
		return permissionWordingZH.MatchString(text)
	case "en":
		return permissionWordingEN.MatchString(text)
	}
	return true // unknown language: never clear a value we cannot judge
}

// D15: performer is not the authority.
//
// The evidence gate only guarantees "this sentence appears in the source". It
// cannot tell that "freeze must be done by the branch manager Zhang San" is an
// executor, not an authorization. So authorization gets one extra check: the
// quoted sentence (±1 sentence) must contain permission wording, otherwise the
// value is cleared and reported so a human can restore it.
//
// Clearing a wrong value costs a reviewer one glance; keeping it can let an action
// run on a misread document. Only authorization is treated this way: the other
// five fields are not permission statements and would be clipped by the same rule.
func ClearPerformerAsAuthorization(contract ActionContract, sourceText string) (ActionContract, bool) {
	field, ok := contract.Field(AuthorizationField)
	if !ok || contractValueEmpty(field.V) {
		return contract, false
	}
	quote := strings.TrimSpace(field.Q)
	if quote == "" {
		// Manual value: there is no source sentence to judge, and the operator
		// already signed off on it.
		return contract, false
	}
	lang := ContractTextLang(quote)
	if lang == "other" {
		// Unknown language: prefer leaving a wrong value for review over deleting a
		// right one.
		return contract, false
	}
	window := contractualSentenceWindow(sourceText, quote)
	if window == "" {
		window = quote
	}
	if hasPermissionWording(window) {
		return contract, false
	}
	out := contract.Clone()
	delete(out.fields, AuthorizationField)
	return out, true
}

// contractualSentenceWindow returns the quoted sentence plus its neighbours.
//
// The neighbours matter because the permission wording often sits in the clause
// before the action ("经支行主管批准后，冻结…"), while the quote is the action
// itself.
//
// The scan is rune-based: slicing a UTF-8 buffer at byte offsets that came from a
// byte search is safe only if the boundaries themselves are ASCII, and Chinese
// punctuation is not. Working on []rune keeps "±1 sentence" exact for both.
func contractualSentenceWindow(sourceText, quote string) string {
	source := []rune(strings.TrimSpace(sourceText))
	needle := []rune(strings.TrimSpace(quote))
	if len(source) == 0 || len(needle) == 0 {
		return ""
	}
	idx := runeIndex(source, needle)
	if idx < 0 {
		// The quote is not in this chunk (the gate ran against another one); the
		// caller cannot judge, so hand back the quote alone.
		return string(needle)
	}
	start := sentenceStart(source, idx)
	// One sentence before the quote's sentence.
	if start > 0 {
		start = sentenceStart(source, start-1)
	}
	end := idx + len(needle)
	for end < len(source) && !isSentenceBoundary(source[end]) {
		end++
	}
	if end < len(source) {
		end++ // include the boundary itself
	}
	// One sentence after.
	for end < len(source) && !isSentenceBoundary(source[end]) {
		end++
	}
	if end < len(source) {
		end++
	}
	return string(source[start:end])
}

func runeIndex(haystack, needle []rune) int {
	if len(needle) == 0 || len(needle) > len(haystack) {
		return -1
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		match := true
		for j := range needle {
			if haystack[i+j] != needle[j] {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}

// sentenceStart walks back to the first rune after the previous boundary.
func sentenceStart(runes []rune, idx int) int {
	for idx > 0 && !isSentenceBoundary(runes[idx-1]) {
		idx--
	}
	return idx
}

func isSentenceBoundary(r rune) bool {
	switch r {
	case '.', '!', '?', '\n', '\r', '。', '！', '？', '；', ';':
		return true
	}
	return false
}

// ---------------------------------------------------------------------------
// Verdict
// ---------------------------------------------------------------------------

// ActionGateInput is everything the four gates need.
//
// It is a plain value so the same evaluation runs in the REST dry-run, in the
// review UI's explanation and in the agent tool, and so tests cover the matrix
// without an engine.
type ActionGateInput struct {
	// ActionExists is false when the name does not resolve to a compiled action.
	ActionExists bool
	// TemplatePublished is false while the model is a draft: its rows were compiled
	// by an older version, so its contract is not the current statement.
	TemplatePublished bool
	// Bound is false when no binding exists for this action.
	Bound bool
	// BindingEnabled is the operator's per-binding switch.
	BindingEnabled bool
	// ToolAvailable is false when the MCP server or the tool disappeared.
	ToolAvailable bool
	// QACallable is the operator's "the agent may call this" switch (default off).
	QACallable bool
	// ReadOnly is declared by the binding, and defaults to false (a write operation)
	// because "declare read-only" proves presence, not absence.
	ReadOnly bool
	// ExecEnabled is the process-wide kill switch (default off).
	ExecEnabled bool
	// Contract is the action's contract.
	Contract ActionContract
}

// ActionGateResult is the verdict and the reasons behind it.
type ActionGateResult struct {
	Verdict ContractVerdict `json:"verdict"`
	Reasons []string        `json:"reasons"`
}

// EvaluateActionGates applies the four gates of the design and returns the
// strictest verdict with every reason that contributed.
//
// Gates (all must pass for `allow`):
//  1. the process switch is on and the binding is qa_callable
//  2. the binding is enabled and the tool resolves
//  3. the contract verdict is allow (an unanswered authorization is manual, not
//     "unrestricted")
//  4. the binding is read-only
//
// refuse > manual > confirm > allow: several failures report the most restrictive
// one, so an operator fixes the worst problem first and a partially configured
// binding can never be reported as merely "confirm".
func EvaluateActionGates(in ActionGateInput) ActionGateResult {
	verdict := ContractVerdictAllow
	reasons := make([]string, 0, 4)
	note := func(v ContractVerdict, codes ...string) {
		if contractStrictness[v] > contractStrictness[verdict] {
			verdict = v
		}
		reasons = append(reasons, codes...)
	}

	if !in.ActionExists {
		note(ContractVerdictRefuse, VerdictReasonActionNotFound)
	}
	if !in.TemplatePublished {
		note(ContractVerdictRefuse, VerdictReasonTemplateUnpublished)
	}
	if !in.Bound {
		note(ContractVerdictManual, VerdictReasonUnbound)
	} else {
		if !in.BindingEnabled {
			note(ContractVerdictManual, VerdictReasonBindingDisabled)
		}
		if !in.ToolAvailable {
			note(ContractVerdictRefuse, VerdictReasonToolUnavailable)
		}
	}
	if !in.ExecEnabled {
		note(ContractVerdictManual, VerdictReasonExecDisabled)
	} else if !in.QACallable {
		note(ContractVerdictManual, VerdictReasonNotQACallable)
	}
	if !in.ReadOnly {
		note(ContractVerdictManual, VerdictReasonNotReadOnly)
	}

	switch in.Contract.Status(AuthorizationField) {
	case ContractStatusMissing:
		note(ContractVerdictManual, VerdictReasonAuthorizationMissing)
	case ContractStatusPresent:
		note(ContractVerdictConfirm, VerdictReasonAuthorizationPresent)
	case ContractStatusExplicitNone:
		// The only state that can reach allow: the document says nobody needs to
		// approve, so there is nothing for a human to confirm.
	}

	if in.Contract.Status("preconditions") != ContractStatusMissing {
		note(ContractVerdictConfirm, VerdictReasonPreconditionsPresent)
	}

	if len(reasons) == 0 {
		reasons = append(reasons, VerdictReasonAllowed)
	}
	return ActionGateResult{Verdict: verdict, Reasons: reasons}
}

// ActionQueryVerdict is the verdict for the question-answering path.
//
// It differs from the manual path in one way: `confirm` is refused, because in a
// chat turn there is nobody to confirm with. The distinction is the whole reason
// the verdict is computed server-side rather than described to the model.
func ActionQueryVerdict(result ActionGateResult) ContractVerdict {
	if result.Verdict == ContractVerdictAllow {
		return ContractVerdictAllow
	}
	if result.Verdict == ContractVerdictConfirm {
		// A human could confirm this; the agent cannot.
		return ContractVerdictConfirm
	}
	return result.Verdict
}

// ActionCallableFromQA reports whether the agent may invoke the action now.
func ActionCallableFromQA(result ActionGateResult) bool {
	return result.Verdict == ContractVerdictAllow
}
