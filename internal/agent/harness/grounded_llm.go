//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

package harness

import (
	"context"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/schema"

	"gorm.io/gorm"
	"ragflow/internal/agent/chat"
)

// LLM groundedness review (draft review). It reviews each claim's draft report
// against the cited evidence semantically, catching over-claims/relation errors
// that the lexical code-level cross-check cannot see.

// groundedSelectPrompt is the groundedness reviewer system prompt (self-contained).
const groundedSelectPrompt = `You are an answer-groundedness reviewer. For each claim's report (the draft answer), determine whether every assertion is supported by the provided evidence.

A claim's report is GROUNDED only if each of its assertions can be inferred from the evidence. The assertion does NOT need to use the same words as the evidence (semantic paraphrase is fine), but it must NOT:
- assert a fact that is absent from the evidence (likely model prior-injection / hallucination);
- assert a relation or value that contradicts the evidence;
- over-claim beyond what the evidence supports (e.g. the evidence only says a medication was prescribed, but the report claims "the patient is well").

Question: %s

Claim reports to verify:
%s

Evidence (each chunk labeled with an integer ID):
%s

For each claim, classify its assertions:
- SUPPORTED: the evidence explicitly supports it, including a semantic paraphrase.
- UNGROUNDED: the evidence lacks the content, or the claimed relation/value contradicts the evidence, or the assertion over-claims beyond the evidence.

Output format (JSON):
{
    "claims": [
        {
            "claim_id": "c1",
            "grounded": true,
            "ungrounded_assertions": []
        },
        {
            "claim_id": "c2",
            "grounded": false,
            "ungrounded_assertions": [
                {"assertion": "the patient had no adverse reactions", "reason": "the evidence only mentions the medication was prescribed, not the patient's reaction"}
            ]
        }
    ]
}

Requirements:
1. Include EVERY claim in the output (do not skip any claim_id).
2. ` + "`grounded`" + ` is true only if ALL of the claim's assertions are supported.
3. ` + "`ungrounded_assertions`" + ` is empty when ` + "`grounded`" + ` is true; otherwise list each ungrounded assertion with a one-line ` + "`reason`" + `.
4. Prefer identifying genuine over-claims or contradictions over surface-level wording differences — a semantic paraphrase IS supported.`

// GroundedVerdict is a single claim's draft-review result.
type GroundedVerdict struct {
	Grounded   bool
	Ungrounded []string
}

// ClaimReport is one claim's draft report submitted for grounded review.
type ClaimReport struct {
	ClaimID string
	Report  string
}

type groundedClaimResult struct {
	ClaimID              string `json:"claim_id"`
	Grounded             bool   `json:"grounded"`
	UngroundedAssertions []struct {
		Assertion string `json:"assertion"`
		Reason    string `json:"reason"`
	} `json:"ungrounded_assertions"`
}

type groundedReviewResult struct {
	Claims []groundedClaimResult `json:"claims"`
}

// LLMGroundedVerify mirrors Python llm_grounded_verify: for each claim report,
// decide whether it is semantically grounded in the cited evidence. Returns an
// empty map when the LLM review is unavailable (no model, no evidence, or a
// failure) — callers treat that as "no new signal".
func LLMGroundedVerify(ctx context.Context, db *gorm.DB, question string, reports []ClaimReport, kb *Kbinfos, evidenceIDs []int) map[string]GroundedVerdict {
	if len(reports) == 0 {
		return nil
	}
	inv := chat.GetDefaultInvoker()
	if inv == nil {
		return nil
	}
	evidenceMD := renderEvidenceMD(kb, evidenceIDs, narrowKeywords(question))
	if evidenceMD == "" {
		return nil
	}

	var reportsMD strings.Builder
	for _, r := range reports {
		if r.Report != "" {
			reportsMD.WriteString(fmt.Sprintf("Claim %s: %s\n", r.ClaimID, r.Report))
		}
	}
	if reportsMD.Len() == 0 {
		reportsMD.WriteString("(no claims)")
	}

	prompt := fmt.Sprintf(groundedSelectPrompt, question, reportsMD.String(), evidenceMD)
	resp, err := inv.Invoke(ctx, db, chat.Request{
		Messages: []schema.Message{
			{Role: schema.System, Content: prompt},
		},
	})
	if err != nil {
		return nil
	}
	var res groundedReviewResult
	if err := unmarshalModelJSON(resp.Content, &res); err != nil {
		return nil
	}
	out := map[string]GroundedVerdict{}
	for _, item := range res.Claims {
		cid := strings.TrimSpace(item.ClaimID)
		if cid == "" {
			continue
		}
		var ungrounded []string
		for _, u := range item.UngroundedAssertions {
			s := strings.TrimSpace(u.Assertion)
			if s == "" {
				s = strings.TrimSpace(u.Reason)
			}
			if s != "" {
				ungrounded = append(ungrounded, s)
			}
		}
		out[cid] = GroundedVerdict{Grounded: item.Grounded, Ungrounded: ungrounded}
	}
	return out
}
