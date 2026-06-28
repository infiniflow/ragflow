//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//

// exesql_trino_stub.go — minimal implementations of the Trino DSN
// helpers referenced by exesql_trino_test.go. The real
// implementation lands when the trino driver integration moves
// from "registered via self-register" to "explicit DSN
// construction" — a follow-up tracked under the design doc
// Future Work.
//
// What this file provides:
//
//   - trinoDSN(exesqlConnParams) string
//     Constructs a Trino DSN with the format
//     `http(s)://<user>[:<password>]@<host>:<port>?catalog=<cat>&schema=<sch>`
//     (matches what trino-go-client v0.333.0 expects).
//
//   - splitTrinoCatalogSchema(db string) (cat, sch string)
//     Splits a `<db>` argument into a Trino `(catalog, schema)` pair.
//     Python `exesql.py:167-176` five quirks are honoured:
//     1. empty password over TLS is preferred (do not embed it)
//     2. TRINO_USE_TLS=1 flips the scheme to https
//     3. default port is 8080 when zero
//     4. empty username → "ragflow"
//     5. catalog[.schema] parsing: first dot/slash is the
//     catalog separator, remainder (if any) is the schema
//
// These implementations are intentionally minimal — they satisfy
// the test contract (splitTrinoCatalogSchema, DSN prefix) without
// claiming feature parity with the Python side. The "happy path"
// DSN shape test (TestExeSQL_TrinoDSN_HappyPath_BasicShape) uses
// these to validate the wiring; richer provider-specific behaviour
// (auth plugins, custom certs, …) is out of scope.
package tool

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
)

// splitTrinoCatalogSchema parses a Trino database spec into a
// (catalog, schema) pair. The first `.` or `/` is the catalog
// separator; everything after it is the schema (which may itself
// contain dots — the schema namespace is a free-form string).
func splitTrinoCatalogSchema(db string) (catalog, schema string) {
	if db == "" {
		return "", ""
	}
	// Honour both `.` and `/` as catalog separators (Python
	// exesql.py:175-176 supports both). The first occurrence wins.
	for _, sep := range []string{".", "/"} {
		if i := strings.Index(db, sep); i >= 0 {
			return db[:i], db[i+1:]
		}
	}
	return db, "default"
}

// trinoDSN builds a Trino DSN from the project's exesql connection
// params. Honours the Python-side quirks documented in
// design doc §10.1 + gap analysis §11.4.1 row 5c.
func trinoDSN(p exesqlConnParams) string {
	scheme := "http"
	if os.Getenv("TRINO_USE_TLS") != "" {
		scheme = "https"
	}
	port := p.Port
	if port == 0 {
		port = 8080
	}
	username := p.Username
	if username == "" {
		username = "ragflow"
	}
	host := p.Host
	if host == "" {
		host = "localhost"
	}
	catalog, schema := splitTrinoCatalogSchema(p.Database)
	if schema == "" {
		schema = "default"
	}
	var user *url.Userinfo
	if scheme == "https" && p.Password != "" {
		user = url.UserPassword(username, p.Password)
	} else {
		user = url.User(username)
	}
	q := url.Values{}
	q.Set("catalog", catalog)
	q.Set("schema", schema)
	u := url.URL{
		Scheme:   scheme,
		User:     user,
		Host:     host + ":" + strconv.Itoa(port),
		RawQuery: q.Encode(),
	}
	// Over plain HTTP, omit the password entirely so the DSN never
	// leaks cleartext credentials in userinfo.
	return u.String()
}

// formatTrinoDSNForLog is a small helper exposed for tests that
// need to assert a DSN without the password portion leaking into
// test logs. Not used by production code.
func formatTrinoDSNForLog(dsn string) string {
	u, err := url.Parse(dsn)
	if err != nil || u.User == nil {
		return dsn
	}
	if _, hasPass := u.User.Password(); hasPass {
		u.User = url.User(u.User.Username())
	}
	if u.RawQuery != "" {
		return fmt.Sprintf("%s://%s@%s?%s", u.Scheme, u.User, u.Host, u.RawQuery)
	}
	return fmt.Sprintf("%s://%s@%s", u.Scheme, u.User, u.Host)
}
