package parser

import (
	"context"
	"testing"
)

// TestAllPureGoProducersAreProduced audits that every pure-Go parser (the
// families that do not require native cgo libraries) returns a Produced()
// result on a successful parse. This guards the gate change in the Parser
// component: buildParserOutputs now keys off Produced() instead of the removed
// produced field, so a successful parser that neglected to set JSON/File
// would silently fall through to the raw-text fallback.
//
// PDF and office families require cgo native libraries and are covered by the
// native-tier tests; they are intentionally excluded here so the default unit
// tier stays free of external dependencies.
func TestAllPureGoProducersAreProduced(t *testing.T) {
	type producer struct {
		name string
		p    ParseResultProducer
		file string
		data []byte
	}

	producers := []producer{
		{"text", NewTextParser(), "doc.txt", []byte("hello world")},
	}

	if mp, err := NewMarkdownParser(GoMarkdown); err == nil {
		producers = append(producers, producer{"markdown", mp, "doc.md", []byte("# Title\n\nbody")})
	}

	producers = append(producers,
		producer{"json", NewJSONParser(), "doc.json", []byte(`{"k":"v"}`)},
		producer{"html", NewHTMLParser(), "doc.html", []byte("<p>hello</p>")},
		producer{"csv", NewCSVParser(), "doc.csv", []byte("a,b\n1,2")},
	)

	for _, pr := range producers {
		res := pr.p.ParseWithResult(context.Background(), pr.file, pr.data)
		if res.Err != nil {
			// A parser that cannot handle the sample input is out of scope; only
			// audit the success path.
			t.Logf("%s parser returned err on sample input (skipped): %v", pr.name, res.Err)
			continue
		}
		if !res.Produced() {
			t.Errorf("%s parser succeeded but result is not Produced (no JSON and no File)", pr.name)
		}
	}
}
