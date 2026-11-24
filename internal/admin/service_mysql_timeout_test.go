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

package admin

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"ragflow/internal/ingestion/testutil"
	"ragflow/internal/server"
)

var probeDriverSeq atomic.Int64

// deadlinePingConn records the deadline carried by the Ping context and
// returns a configurable error, so timeout behavior is asserted directly
// instead of waiting on a wall clock.
type deadlinePingConn struct {
	deadline atomic.Int64 // nanoseconds remaining; 0 means no deadline
	err      error
}

func (*deadlinePingConn) Prepare(string) (driver.Stmt, error) {
	return nil, fmt.Errorf("not supported")
}
func (*deadlinePingConn) Close() error              { return nil }
func (*deadlinePingConn) Begin() (driver.Tx, error) { return nil, fmt.Errorf("not supported") }

func (c *deadlinePingConn) Ping(ctx context.Context) error {
	if dl, ok := ctx.Deadline(); ok {
		c.deadline.Store(int64(time.Until(dl)))
	} else {
		c.deadline.Store(0)
	}
	return c.err
}

type deadlinePingDriver struct{ conn *deadlinePingConn }

func (d *deadlinePingDriver) Open(string) (driver.Conn, error) { return d.conn, nil }

// installDeadlineProbe replaces dao.DB with a DB whose Ping records its
// context deadline. Returns the connection so tests can inspect the deadline
// getMySQLStatus applied.
func installDeadlineProbe(t *testing.T, pingErr error) *deadlinePingConn {
	t.Helper()
	if server.GetConfig() == nil {
		if err := server.Init(""); err != nil {
			t.Fatalf("server.Init: %v", err)
		}
	}
	conn := &deadlinePingConn{}
	name := fmt.Sprintf("deadline-probe-%d", probeDriverSeq.Add(1))
	sql.Register(name, &deadlinePingDriver{conn: conn})
	sqlDB, err := sql.Open(name, "")
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	gormDB, err := gorm.Open(mysql.New(mysql.Config{
		Conn:                      sqlDB,
		SkipInitializeWithVersion: true,
	}), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm.Open: %v", err)
	}
	restore := testutil.ReplaceDBForTest(t, gormDB)
	t.Cleanup(restore)
	// gorm.Open pings during initialization; arm the error only after that so
	// the failure is observed by getMySQLStatus rather than the dialector.
	conn.err = pingErr
	return conn
}

// TestGetMySQLStatus_PingTimeout is the primary RED test: getMySQLStatus
// previously passed the request context straight through, so an unreachable
// MySQL inherited the DSN's 30s dial timeout instead of a 3s cap.
func TestGetMySQLStatus_PingTimeout(t *testing.T) {
	conn := installDeadlineProbe(t, nil)
	st := (&Service{}).getMySQLStatus(context.Background())
	if st.Status != "alive" {
		t.Fatalf("status = %q, want alive", st.Status)
	}
	dl := time.Duration(conn.deadline.Load())
	if dl <= 2*time.Second || dl > 3*time.Second {
		t.Fatalf("ping deadline = %v, want ~3s", dl)
	}
}

// TestGetMySQLStatus_RespectsShorterCallerDeadline ensures the 3s cap does not
// extend a caller deadline that is already shorter.
func TestGetMySQLStatus_RespectsShorterCallerDeadline(t *testing.T) {
	conn := installDeadlineProbe(t, nil)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = (&Service{}).getMySQLStatus(ctx)
	dl := time.Duration(conn.deadline.Load())
	if dl <= 0 || dl > time.Second {
		t.Fatalf("ping deadline = %v, want <= caller's 1s", dl)
	}
}

// TestGetMySQLStatus_PingFailureKeepsTimeoutStatus pins the status text on a
// failed probe so the timeout fix changes no observable response.
func TestGetMySQLStatus_PingFailureKeepsTimeoutStatus(t *testing.T) {
	conn := installDeadlineProbe(t, context.DeadlineExceeded)
	st := (&Service{}).getMySQLStatus(context.Background())
	if st.Status != "timeout" {
		t.Fatalf("status = %q, want timeout", st.Status)
	}
	dl := time.Duration(conn.deadline.Load())
	if dl <= 2*time.Second || dl > 3*time.Second {
		t.Fatalf("ping deadline = %v, want ~3s", dl)
	}
}
