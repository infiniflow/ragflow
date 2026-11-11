package contract

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParseActionContractEmptyIsAllMissing(t *testing.T) {
	for _, raw := range []any{nil, "", "  ", "null", "{}", map[string]any{}, []byte("{}")} {
		contract := ParseActionContract(raw)
		if !contract.IsEmpty() {
			t.Fatalf("%v: expected empty contract", raw)
		}
		if got := len(contract.MissingFields()); got != len(actionContractFields) {
			t.Fatalf("%v: expected %d missing fields, got %d", raw, len(actionContractFields), got)
		}
		if contract.Status(AuthorizationField) != ContractStatusMissing {
			t.Fatalf("%v: empty authorization must be missing, not %q", raw, contract.Status(AuthorizationField))
		}
	}
}

func TestParseActionContractFlatShape(t *testing.T) {
	raw := `{"_v":1,"authorization":{"v":"需支行主管授权","q":"冻结前须经支行主管授权。"},
		"audit_required":{"v":true},"preconditions":{"v":[]}}`
	contract := ParseActionContract(raw)
	if contract.Malformed() {
		t.Fatal("valid json reported malformed")
	}
	if got := contract.Value(AuthorizationField); got != "需支行主管授权" {
		t.Fatalf("authorization value = %v", got)
	}
	if got := contract.Quote(AuthorizationField); got != "冻结前须经支行主管授权。" {
		t.Fatalf("authorization quote = %q", got)
	}
	// Boolean false/true are values, not absences: "no audit required" is a statement.
	if got := contract.Status("audit_required"); got != ContractStatusPresent {
		t.Fatalf("audit_required status = %q, want present", got)
	}
	// An empty list is the same as no value.
	if got := contract.Status("preconditions"); got != ContractStatusMissing {
		t.Fatalf("empty preconditions status = %q, want missing", got)
	}
}

func TestParseActionContractBareScalarReadsAsManual(t *testing.T) {
	contract := ParseActionContract(map[string]any{"authorization": "支行主管"})
	if got := contract.Provenance(AuthorizationField); got != ContractProvenanceManual {
		t.Fatalf("provenance = %q, want manual", got)
	}
}

func TestParseActionContractMalformedStaysSafe(t *testing.T) {
	contract := ParseActionContract("{not json")
	if !contract.Malformed() {
		t.Fatal("expected malformed flag")
	}
	if contract.Status(AuthorizationField) != ContractStatusMissing {
		t.Fatal("malformed contract must read as missing, never as allowed")
	}
}

func TestContractStatusExplicitNoneIsLanguageScoped(t *testing.T) {
	zh := NewActionContract()
	zh.SetManual(AuthorizationField, "无需授权")
	if got := zh.Status(AuthorizationField); got != ContractStatusExplicitNone {
		t.Fatalf("zh explicit none = %q", got)
	}

	en := NewActionContract()
	en.SetManual(AuthorizationField, "No approval required.")
	if got := en.Status(AuthorizationField); got != ContractStatusExplicitNone {
		t.Fatalf("en explicit none = %q", got)
	}

	// Chinese patterns must not be applied to German text: an unmatched pattern is
	// invisible, while a wrong match turns into "no approval required".
	de := NewActionContract()
	de.SetManual(AuthorizationField, "Keine Autorisierung erforderlich")
	if got := de.Status(AuthorizationField); got != ContractStatusPresent {
		t.Fatalf("other-language value = %q, want present", got)
	}

	// A value that merely mentions approval is present, not explicit_none.
	present := NewActionContract()
	present.SetManual(AuthorizationField, "须经支行主管审批")
	if got := present.Status(AuthorizationField); got != ContractStatusPresent {
		t.Fatalf("present = %q", got)
	}
}

func TestContractProvenanceDerivesFromQuote(t *testing.T) {
	contract := NewActionContract()
	contract.Set(AuthorizationField, "需支行主管授权", "冻结前须经支行主管授权。")
	contract.SetManual("preconditions", "卡片状态正常")

	cases := map[string]ContractProvenance{
		AuthorizationField: ContractProvenanceDocument,
		"preconditions":    ContractProvenanceManual,
		"atomic_effects":   ContractProvenanceAbsent,
	}
	for name, want := range cases {
		if got := contract.Provenance(name); got != want {
			t.Fatalf("%s provenance = %q, want %q", name, got, want)
		}
	}
}

func TestContractSetManualDropsQuote(t *testing.T) {
	contract := NewActionContract()
	contract.Set(AuthorizationField, "需支行主管授权", "冻结前须经支行主管授权。")
	contract.SetManual(AuthorizationField, "支行主管")
	if got := contract.Quote(AuthorizationField); got != "" {
		t.Fatalf("manual edit must delete the quote, got %q", got)
	}
	if got := contract.Provenance(AuthorizationField); got != ContractProvenanceManual {
		t.Fatalf("provenance = %q", got)
	}
}

func TestContractMergeExtractedProtectsManualValues(t *testing.T) {
	existing := NewActionContract()
	existing.SetManual(AuthorizationField, "支行主管")      // manual
	existing.Set("preconditions", "状态正常", "卡片状态须为正常。")  // documented
	existing.Set("failure_policy", "失败回滚", "失败时回滚并记录。") // documented

	next := NewActionContract()
	next.Set(AuthorizationField, "分行行长", "")                // no quote: must not win
	next.Set("preconditions", "状态为正常且未挂失", "卡片状态须为正常且未挂失。") // quote: wins
	// atomic_effects absent in next: existing absent stays absent.

	merged := existing.MergeExtracted(next)
	if got := merged.Value(AuthorizationField); got != "支行主管" {
		t.Fatalf("manual value overwritten: %v", got)
	}
	if got := merged.Value("preconditions"); got != "状态为正常且未挂失" {
		t.Fatalf("documented value not refreshed: %v", got)
	}
	if got := merged.Value("failure_policy"); got != "失败回滚" {
		t.Fatalf("untouched field lost: %v", got)
	}
	if got := merged.Status("atomic_effects"); got != ContractStatusMissing {
		t.Fatalf("atomic_effects = %q", got)
	}
}

func TestContractViewReportsSixFields(t *testing.T) {
	contract := NewActionContract()
	contract.Set(AuthorizationField, "需支行主管授权", "冻结前须经支行主管授权。")
	contract.SetManual("audit_required", true)

	view := contract.View()
	if len(view.Fields) != len(actionContractFields) {
		t.Fatalf("view fields = %d, want %d", len(view.Fields), len(actionContractFields))
	}
	if view.Declared != 2 {
		t.Fatalf("declared = %d, want 2", view.Declared)
	}
	if len(view.Missing) != 4 {
		t.Fatalf("missing = %v", view.Missing)
	}
	if got := view.Fields["atomic_effects"].Status; got != ContractStatusMissing {
		t.Fatalf("atomic_effects status = %q", got)
	}
}

func TestContractJSONRoundTrip(t *testing.T) {
	contract := NewActionContract()
	contract.Set(AuthorizationField, "需支行主管授权", "冻结前须经支行主管授权。")
	contract.SetManual("audit_required", true)

	raw, err := json.Marshal(contract)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(raw), `"_v":1`) {
		t.Fatalf("missing structure version: %s", raw)
	}
	back := ParseActionContract(raw)
	if got := back.Value(AuthorizationField); got != "需支行主管授权" {
		t.Fatalf("round trip value = %v", got)
	}
	if got := back.Quote(AuthorizationField); got != "冻结前须经支行主管授权。" {
		t.Fatalf("round trip quote = %q", got)
	}
	if got := back.Provenance("audit_required"); got != ContractProvenanceManual {
		t.Fatalf("round trip provenance = %q", got)
	}
}

func TestClearPerformerAsAuthorization(t *testing.T) {
	// The classic D15 failure: the sentence names the person who carries the step
	// out, and the extractor filed it as who may authorize it.
	contract := NewActionContract()
	contract.Set(AuthorizationField, "支行主管张三", "冻结须由支行主管张三操作。")
	cleared, changed := ClearPerformerAsAuthorization(contract, "冻结须由支行主管张三操作。卡片状态须为正常。")
	if !changed {
		t.Fatal("performer-as-authorization was not cleared")
	}
	if cleared.Status(AuthorizationField) != ContractStatusMissing {
		t.Fatalf("authorization = %q, want missing after D15", cleared.Status(AuthorizationField))
	}
	// The correction is scoped to authorization: other fields stay as they were.
	if got := cleared.DeclaredCount(); got != 0 {
		t.Fatalf("unexpected remaining fields: %d", got)
	}

	// Real authorization wording one sentence earlier must keep the value.
	keep := NewActionContract()
	keep.Set(AuthorizationField, "支行主管", "冻结前须经支行主管授权。")
	kept, changed := ClearPerformerAsAuthorization(keep, "冻结前须经支行主管授权。冻结后短信通知持卡人。")
	if changed {
		t.Fatal("value with permission wording must not be cleared")
	}
	if got := kept.Value(AuthorizationField); got != "支行主管" {
		t.Fatalf("value = %v", got)
	}

	// English documents get the same treatment.
	en := NewActionContract()
	en.Set(AuthorizationField, "branch manager", "The card is frozen by the branch manager.")
	if _, changed := ClearPerformerAsAuthorization(en, "The card is frozen by the branch manager."); !changed {
		t.Fatal("english performer-as-authorization was not cleared")
	}

	// Manual values have no source sentence and must survive untouched.
	manual := NewActionContract()
	manual.SetManual(AuthorizationField, "支行主管")
	if _, changed := ClearPerformerAsAuthorization(manual, "任何文本"); changed {
		t.Fatal("manual value must not be cleared by D15")
	}

	// Unknown languages: leave it to a human rather than delete a possibly right value.
	other := NewActionContract()
	other.Set(AuthorizationField, "Filialleiter", "Die Karte wird vom Filialleiter gesperrt.")
	if _, changed := ClearPerformerAsAuthorization(other, "Die Karte wird vom Filialleiter gesperrt."); changed {
		t.Fatal("other-language value must not be cleared")
	}
}

func TestActionGatesAllPassIsAllow(t *testing.T) {
	contract := NewActionContract()
	contract.SetManual(AuthorizationField, "无需授权")
	result := EvaluateActionGates(ActionGateInput{
		ActionExists:      true,
		TemplatePublished: true,
		Bound:             true,
		BindingEnabled:    true,
		ToolAvailable:     true,
		QACallable:        true,
		ReadOnly:          true,
		ExecEnabled:       true,
		Contract:          contract,
	})
	if result.Verdict != ContractVerdictAllow {
		t.Fatalf("verdict = %q reasons = %v", result.Verdict, result.Reasons)
	}
	if len(result.Reasons) != 1 || result.Reasons[0] != VerdictReasonAllowed {
		t.Fatalf("reasons = %v", result.Reasons)
	}
}

func TestActionGatesStrictnessOrdering(t *testing.T) {
	allow := NewActionContract()
	allow.SetManual(AuthorizationField, "无需授权")

	cases := []struct {
		name string
		in   ActionGateInput
		want ContractVerdict
		has  string
	}{
		{
			name: "missing authorization is manual not allow",
			in: ActionGateInput{ActionExists: true, TemplatePublished: true, Bound: true,
				BindingEnabled: true, ToolAvailable: true, QACallable: true, ReadOnly: true,
				ExecEnabled: true, Contract: NewActionContract()},
			want: ContractVerdictManual,
			has:  VerdictReasonAuthorizationMissing,
		},
		{
			name: "present authorization asks for confirmation",
			in: ActionGateInput{ActionExists: true, TemplatePublished: true, Bound: true,
				BindingEnabled: true, ToolAvailable: true, QACallable: true, ReadOnly: true,
				ExecEnabled: true, Contract: func() ActionContract {
					c := NewActionContract()
					c.Set(AuthorizationField, "支行主管", "须经支行主管授权。")
					return c
				}()},
			want: ContractVerdictConfirm,
			has:  VerdictReasonAuthorizationPresent,
		},
		{
			name: "preconditions force confirm even when authorization is explicitly none",
			in: ActionGateInput{ActionExists: true, TemplatePublished: true, Bound: true,
				BindingEnabled: true, ToolAvailable: true, QACallable: true, ReadOnly: true,
				ExecEnabled: true, Contract: func() ActionContract {
					c := NewActionContract()
					c.SetManual(AuthorizationField, "无需授权")
					c.SetManual("preconditions", "卡片状态正常")
					return c
				}()},
			want: ContractVerdictConfirm,
			has:  VerdictReasonPreconditionsPresent,
		},
		{
			name: "unbound is manual",
			in: ActionGateInput{ActionExists: true, TemplatePublished: true,
				ExecEnabled: true, Contract: allow},
			want: ContractVerdictManual,
			has:  VerdictReasonUnbound,
		},
		{
			name: "unpublished template refuses and outranks manual",
			in:   ActionGateInput{ActionExists: true, Contract: NewActionContract()},
			want: ContractVerdictRefuse,
			has:  VerdictReasonTemplateUnpublished,
		},
		{
			name: "unknown action refuses",
			in:   ActionGateInput{TemplatePublished: true, ExecEnabled: true, Contract: allow},
			want: ContractVerdictRefuse,
			has:  VerdictReasonActionNotFound,
		},
		{
			name: "write operation is manual",
			in: ActionGateInput{ActionExists: true, TemplatePublished: true, Bound: true,
				BindingEnabled: true, ToolAvailable: true, QACallable: true,
				ExecEnabled: true, Contract: allow},
			want: ContractVerdictManual,
			has:  VerdictReasonNotReadOnly,
		},
		{
			name: "exec switch off is manual",
			in: ActionGateInput{ActionExists: true, TemplatePublished: true, Bound: true,
				BindingEnabled: true, ToolAvailable: true, QACallable: true, ReadOnly: true,
				Contract: allow},
			want: ContractVerdictManual,
			has:  VerdictReasonExecDisabled,
		},
	}

	for _, tc := range cases {
		got := EvaluateActionGates(tc.in)
		if got.Verdict != tc.want {
			t.Fatalf("%s: verdict = %q, want %q (reasons %v)", tc.name, got.Verdict, tc.want, got.Reasons)
		}
		found := false
		for _, reason := range got.Reasons {
			if reason == tc.has {
				found = true
			}
		}
		if !found {
			t.Fatalf("%s: reasons %v missing %q", tc.name, got.Reasons, tc.has)
		}
	}
}

func TestActionQueryVerdictRefusesConfirm(t *testing.T) {
	result := ActionGateResult{Verdict: ContractVerdictConfirm}
	if got := ActionQueryVerdict(result); got != ContractVerdictConfirm {
		t.Fatalf("query verdict = %q", got)
	}
	if ActionCallableFromQA(result) {
		t.Fatal("confirm must not be callable from the QA path")
	}
	if !ActionCallableFromQA(ActionGateResult{Verdict: ContractVerdictAllow}) {
		t.Fatal("allow must be callable")
	}
}

// A template may narrow the contract. The declaration then decides what is owed:
// judging a narrowed contract against all six would report every action of that
// model as permanently incomplete, and the reviewer would learn to ignore the
// missing list entirely.
func TestDeclaredNarrowingDecidesWhatIsMissing(t *testing.T) {
	c := NewActionContract()
	c.SetDeclared([]string{"authorization", "audit_required"})
	c.Set("authorization", "the compliance officer", "Compliance must approve.")
	c.Set("audit_required", true, "The decision is logged.")

	if got := c.RequiredFields(); strings.Join(got, ",") != "authorization,audit_required" {
		t.Fatalf("required = %v", got)
	}
	if got := c.MissingFields(); len(got) != 0 {
		t.Fatalf("a fully answered narrow contract still reports missing fields: %v", got)
	}
	if got := c.DeclaredCount(); got != 2 {
		t.Fatalf("declared = %d, want 2", got)
	}
	view := c.View()
	if len(view.Required) != 2 || view.Declared != 2 {
		t.Fatalf("view = required %v declared %d", view.Required, view.Declared)
	}
	// The value of an unrequired field is still reported: it is real data, and the
	// operator who has to edit it must be able to see it.
	if view.Fields["preconditions"].Status != ContractStatusMissing {
		t.Fatalf("preconditions = %q", view.Fields["preconditions"].Status)
	}
}

func TestNormalizeDeclaredRules(t *testing.T) {
	cases := []struct {
		name   string
		in     []string
		expect string
	}{
		{"padding and case", []string{" authorization ", "follow_ups"}, "authorization,follow_ups"},
		{"unknown dropped", []string{"authorization", "parameters"}, "authorization"},
		{"duplicate collapsed", []string{"authorization", "authorization"}, "authorization"},
		{"authorization prepended", []string{"follow_ups", "audit_required"}, "authorization,follow_ups,audit_required"},
		{"declaration order kept", []string{"audit_required", "authorization"}, "audit_required,authorization"},
		{"nothing declared", nil, strings.Join(actionContractFields, ",")},
		{"nothing recognisable", []string{"parameters", "definition"}, strings.Join(actionContractFields, ",")},
		{"all six", actionContractFields, strings.Join(actionContractFields, ",")},
	}
	for _, tc := range cases {
		if got := strings.Join(NormalizeDeclared(tc.in), ","); got != tc.expect {
			t.Fatalf("%s: NormalizeDeclared(%v) = %q, want %q", tc.name, tc.in, got, tc.expect)
		}
	}
}

// The declaration travels in the column, because the review queue, the action
// workbench and the agent tool all read the row and none of them loads the
// template that produced it.
func TestDeclaredSurvivesTheColumn(t *testing.T) {
	c := NewActionContract()
	c.SetDeclared([]string{"authorization", "audit_required"})
	c.Set("authorization", "the compliance officer", "Compliance must approve.")

	encoded, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	parsed := ParseActionContract(string(encoded))
	if got := strings.Join(parsed.RequiredFields(), ","); got != "authorization,audit_required" {
		t.Fatalf("declaration lost in the column: %q (column %s)", got, encoded)
	}
	if _, ok := parsed.Field("_d"); ok {
		t.Fatalf("the declaration leaked into the fields: %s", encoded)
	}
	if got := parsed.MissingFields(); len(got) != 1 || got[0] != "audit_required" {
		t.Fatalf("missing = %v", got)
	}

	// A row compiled before the declaration existed reads as all six, and a wide
	// declaration round-trips as all six too.
	for _, column := range []string{`{"_v":1,"authorization":{"v":"x","q":"y"}}`, `{"_v":1,"_d":["authorization","preconditions","atomic_effects","follow_ups","failure_policy","audit_required"]}`} {
		if got := len(ParseActionContract(column).RequiredFields()); got != len(actionContractFields) {
			t.Fatalf("%s: required = %d, want %d", column, got, len(actionContractFields))
		}
	}
}

// A recompile is the moment a re-declared contract takes effect; an extraction
// that carries no declaration predates one and must not clear it.
func TestMergeExtractedAdoptsAndKeepsTheDeclaration(t *testing.T) {
	stored := NewActionContract()
	stored.Set("authorization", "the compliance officer", "Compliance must approve.")

	narrow := NewActionContract()
	narrow.SetDeclared([]string{"authorization"})
	narrow.Set("authorization", "the risk officer", "Risk must approve.")
	merged := stored.MergeExtracted(narrow)
	if got := strings.Join(merged.RequiredFields(), ","); got != "authorization" {
		t.Fatalf("re-declaration not adopted: %q", got)
	}
	if merged.Value("authorization") != "the risk officer" {
		t.Fatalf("quoted extraction did not win: %v", merged.Value("authorization"))
	}

	legacy := NewActionContract()
	legacy.Set("authorization", "someone", "")
	kept := merged.MergeExtracted(legacy)
	if got := strings.Join(kept.RequiredFields(), ","); got != "authorization" {
		t.Fatalf("a declaration-less extraction cleared the declaration: %q", got)
	}
	if kept.Value("authorization") != "the risk officer" {
		t.Fatalf("a manual value overwrote a quoted one: %v", kept.Value("authorization"))
	}
}
