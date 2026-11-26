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

package opengauss

import (
	"context"
	"errors"
	"net"
	"reflect"
	"strings"
	"testing"
	"time"

	"ragflow/internal/server/config"
)

func TestScopedReportShapes(t *testing.T) {
	tables := map[string][]string{"ragflow_reports": {"east"}}
	for _, query := range []string{
		"SELECT id FROM (SELECT id FROM ragflow_reports) items",
		"SELECT (SELECT 1) FROM ragflow_reports",
		"WITH items AS (SELECT id FROM ragflow_reports) SELECT id FROM items",
		"SELECT id FROM ragflow_other",
		"SELECT public.lower(id) FROM ragflow_reports",
		"SELECT pg_catalog.reverse(id) FROM ragflow_reports",
		"SELECT other_db.pg_catalog.lower(id) FROM ragflow_reports",
	} {
		if _, ok := ScopeChatSQL(query, tables); ok {
			t.Errorf("accepted unsupported report shape %q", query)
		}
	}
	if _, ok := ScopeChatSQL("SELECT\n count(*) AS total FROM ragflow_reports WHERE id = 'A;B' /* one statement */", tables); !ok {
		t.Fatal("simple aggregate rejected")
	}
	query := "SELECT count(*) FROM ragflow_reports WHERE lower(docnm_kwd) = 'notes'"
	for pass := 1; pass <= 2; pass++ {
		scoped, ok := ScopeChatSQL(query, tables)
		if !ok || !strings.Contains(scoped, "pg_catalog.lower(") || !strings.Contains(scoped, "pg_catalog.count(") {
			t.Fatalf("scope pass %d: %q, accepted=%t", pass, scoped, ok)
		}
		query = scoped
	}
}

func TestConnectionOptionsAndHandshake(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	cfg := config.OpenGaussConfig{Host: "127.0.0.1", Port: listener.Addr().(*net.TCPAddr).Port, Database: "postgres", Schema: "public", Password: "quote' " + t.Name(), SSLMode: "invalid"}
	if _, err := openPool(cfg); err == nil || strings.Contains(err.Error(), t.Name()) {
		t.Fatal("invalid connection options must return a credential-free error")
	}
	cfg.SSLMode = "require"
	pool, err := openPool(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 300*time.Millisecond)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- pool.PingContext(ctx) }()
	listener.(*net.TCPListener).SetDeadline(time.Now().Add(2 * time.Second))
	socket, err := listener.Accept()
	if err != nil {
		t.Fatal(err)
	}
	defer socket.Close()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("incomplete handshake succeeded")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("handshake outlived its deadline")
	}
}

func TestDDLWaitCancellation(t *testing.T) {
	lock := make(chan struct{}, 1)
	lock <- struct{}{}
	ddlLocks.Store(t.Name(), lock)
	defer ddlLocks.Delete(t.Name())
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	probe := func() (bool, error) { t.Fatal("cancelled waiter took the active lock"); return false, nil }
	err := new(Engine).withDDLLock(ctx, t.Name(), probe, nil)
	if !errors.Is(err, context.Canceled) || len(lock) != 1 {
		t.Fatalf("cancelled wait: %v", err)
	}
}

func TestMemoryEmbeddingsAndTokenOffsets(t *testing.T) {
	row := map[string]any{"q_2_vec": "[1,0]", "q_3_vec": "[0,1,0]"}
	for _, tc := range []struct {
		column string
		want   []float64
	}{{"", []float64{1, 0}}, {"q_3_vec", []float64{0, 1, 0}}} {
		if got := new(Engine).decodeLogicalRow(row, "memory", tc.column)["content_embed"]; !reflect.DeepEqual(got, tc.want) {
			t.Errorf("embedding %s: %v", tc.column, got)
		}
	}
	got := findTokenMatches("文 Go K", "文 go k", map[string]struct{}{"go": {}, "k": {}})
	if marked := applyMatches("文 Go K", got); marked != "文 <em>Go</em> <em>K</em>" {
		t.Errorf("marked text = %q", marked)
	}
	for _, bad := range []any{[][]int{{1, 2}, {3, 4, 5}}, []int{1, 2}} {
		if _, err := encodeIntegerArray("position_int", bad); err == nil {
			t.Errorf("accepted invalid position %v", bad)
		}
	}
}
