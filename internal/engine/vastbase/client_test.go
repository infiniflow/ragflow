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

package vastbase

import (
	"context"
	"regexp"
	"testing"

	"ragflow/internal/server/config"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestBuildDSNAssemblesKeywordForm(t *testing.T) {
	t.Setenv("VASTBASE_DSN", "")
	got := buildDSN(config.VastbaseConfig{
		Host: "vb-host", Port: 15432, User: "rag user",
		Password: "p@ss word'", DBName: "rag flow", SSLMode: "verify-full",
	})
	want := `host='vb-host' port=15432 user='rag user' dbname='rag flow' sslmode='verify-full' ` +
		`password='p@ss word\'' connect_timeout=30 statement_timeout=300000`
	if got != want {
		t.Fatalf("DSN = %q", got)
	}
}

func TestBuildDSNDefaultsAndOverride(t *testing.T) {
	t.Setenv("VASTBASE_DSN", "")
	// Unset ssl_mode: the default host is a TCP service name, so the
	// transport policy requires certificate-verified TLS.
	got := buildDSN(config.VastbaseConfig{})
	if !regexp.MustCompile(`^host='vastbase' port=5432 user='ragflow' dbname='ragflow' sslmode='verify-full' connect_timeout=30`).MatchString(got) {
		t.Fatalf("default DSN = %q", got)
	}
	// No password configured: the keyword must be absent, not empty.
	if regexp.MustCompile(`password=''`).MatchString(got) {
		t.Fatalf("empty password must be omitted: %q", got)
	}
	// Loopback and unix sockets may default to plaintext.
	for _, host := range []string{"localhost", "127.0.0.1"} {
		if got := buildDSN(config.VastbaseConfig{Host: host}); !regexp.MustCompile(`sslmode='disable'`).MatchString(got) {
			t.Fatalf("local host %s must default to disable: %q", host, got)
		}
	}
	// An explicit ssl_mode is honored for any host.
	if got := buildDSN(config.VastbaseConfig{SSLMode: "require"}); !regexp.MustCompile(`sslmode='require'`).MatchString(got) {
		t.Fatalf("explicit ssl_mode must be honored: %q", got)
	}
	t.Setenv("VASTBASE_DSN", "postgres://u:secret@h:5432/db")
	if got := buildDSN(config.VastbaseConfig{Host: "ignored"}); got != "postgres://u:secret@h:5432/db" {
		t.Fatalf("VASTBASE_DSN override = %q", got)
	}
}

func TestRedactDSN(t *testing.T) {
	keyword := redactDSN(`host='h' password='s3cret' user='u'`)
	if keyword != `host='h' password=*** user='u'` {
		t.Fatalf("keyword redaction = %q", keyword)
	}
	// lib/pq accepts whitespace around "=" — those DSNs must redact too.
	spaced := redactDSN(`host=h user=u password = 'secret' dbname=d`)
	if spaced != `host=h user=u password = *** dbname=d` {
		t.Fatalf("spaced keyword redaction = %q", spaced)
	}
	// Escaped characters survive in unquoted values, including escaped
	// spaces: the whole token is the password, not just its first word.
	escaped := redactDSN(`password=se\ cret dbname=d`)
	if escaped != `password=*** dbname=d` {
		t.Fatalf("escaped unquoted redaction = %q", escaped)
	}
	escapedQuote := redactDSN(`host=h password='a\'b' port=5432`)
	if escapedQuote != `host=h password=*** port=5432` {
		t.Fatalf("escaped quoted redaction = %q", escapedQuote)
	}
	url := redactDSN("postgres://ragflow:Infini_Rag%40123@vastbase:5432/ragflow")
	if url != "postgres://ragflow:***@vastbase:5432/ragflow" {
		t.Fatalf("url redaction = %q", url)
	}
}

func TestDistanceOpPerCompatibility(t *testing.T) {
	if op := (&Engine{dbCompatibility: compatB}).distanceOp(); op != "<+>" {
		t.Fatalf("B-mode operator = %q", op)
	}
	if op := (&Engine{dbCompatibility: compatPG}).distanceOp(); op != "<=>" {
		t.Fatalf("PG-mode operator = %q", op)
	}
	// Anything unrecognized falls back to the PG operator.
	if op := (&Engine{dbCompatibility: "weird"}).distanceOp(); op != "<=>" {
		t.Fatalf("fallback operator = %q", op)
	}
}

func TestGetTypeAndSupportsPageRank(t *testing.T) {
	engine := newEngineWithDB(config.VastbaseConfig{DBCompatibility: compatB}, nil)
	if engine.GetType() != "vastbase" {
		t.Fatalf("engine type = %q", engine.GetType())
	}
	if !engine.SupportsPageRank() {
		t.Fatal("pagerank_fea is a real column; pagerank must be supported")
	}
}

func TestAdjustChunkPagerankSQL(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	engine := newEngineWithDB(config.VastbaseConfig{DBCompatibility: compatPG}, db)

	mock.ExpectExec(regexp.QuoteMeta(
		`UPDATE "t1" SET pagerank_fea = GREATEST($1, LEAST($2, COALESCE(pagerank_fea, 0) + $3)) WHERE id = $4 AND kb_id = $5`)).
		WithArgs(0.0, 10.0, 0.5, "c1", "kb1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := engine.AdjustChunkPagerank(context.Background(), "t1", "c1", "kb1", 0.5, 0, 10); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}

	if err := engine.AdjustChunkPagerank(context.Background(), "bad;table", "c1", "kb1", 0.5, 0, 10); err == nil {
		t.Fatal("invalid table names must be rejected")
	}
}

func TestValidateIdentifier(t *testing.T) {
	for _, name := range []string{"t1", "memory_x", "skill_abc", "ragflow_doc_meta_tenant-1"} {
		if err := validateIdentifier(name); err != nil {
			t.Fatalf("validateIdentifier(%q) = %v", name, err)
		}
	}
	for _, name := range []string{"", `bad"quote`, "space name", "semi;colon"} {
		if err := validateIdentifier(name); err == nil {
			t.Fatalf("validateIdentifier(%q) must fail", name)
		}
	}
}

func TestQuoteIdentEscapesDoubleQuotes(t *testing.T) {
	if got := quoteIdent(`we"ird`); got != `"we""ird"` {
		t.Fatalf("quoteIdent = %q", got)
	}
}
