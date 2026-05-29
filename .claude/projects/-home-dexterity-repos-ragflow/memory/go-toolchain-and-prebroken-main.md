---
name: go-toolchain-and-prebroken-main
description: Where the Go toolchain lives and which parts of the Go build/tests are pre-broken on local main
metadata:
  type: project
---

The Go toolchain is NOT on PATH. It lives at `/tmp/go/bin` (go1.25.0); run `export PATH=$PATH:/tmp/go/bin` first. `gofmt` is `/tmp/go/bin/gofmt`. The Go module is `ragflow`, rooted at the repo root (`go.mod`); the model providers are package `ragflow/internal/entity/models`.

As of 2026-05-28, several things are pre-broken on `main`, independent of any feature work:
- `go build ./...` fails: `cmd/` has multiple `func main()` (admin_server.go, ingestion_server.go, ragflow_cli.go, server_main.go). Build a specific package instead, e.g. `go build ./internal/entity/models/`.
- The `internal/entity/models` test suite did not even compile: `roundTripperFunc` was redeclared across ppio_test.go, longcat_test.go, modelscope_test.go, voyage_test.go (all `package models`). Once it compiles, many `Test*EmbedReturnsNoSuchMethod` / `*BalanceReturnsNoSuchMethod` tests panic with nil `ApiKey` deref (astraflow, hunyuan, novita, cometapi, mistral, longcat, bedrock, tokenpony, ...) and some expectation mismatches exist (e.g. TogetherAI `Rerank` with empty docs returns nil, test wants "no such method"). A panic aborts the whole `go test` run.

To verify a models change, build the package directly and run targeted tests with `-run '<providers>'` (positively selecting providers without the panic family) rather than the whole suite.
