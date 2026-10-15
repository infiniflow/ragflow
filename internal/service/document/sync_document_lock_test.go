package document

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestMySQLSyncDocumentLockAcquisitionOutcomes(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value interface{}
		err   error
	}{
		{"timeout", int64(0), nil},
		{"null", nil, nil},
		{"query error", nil, errors.New("lock query failed")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pool, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { pool.Close() })
			query := mock.ExpectQuery(`SELECT GET_LOCK\(\?, \?\)`).WithArgs("sync-document:test", int64(30))
			if tc.err != nil {
				query.WillReturnError(tc.err)
			} else {
				query.WillReturnRows(sqlmock.NewRows([]string{"granted"}).AddRow(tc.value))
			}
			unlock, err := acquireMySQLSyncDocumentLock(t.Context(), pool, "sync-document:test")
			if err == nil || unlock != nil {
				t.Fatalf("failed acquisition = %v, unlock = %v", err, unlock != nil)
			}
			if tc.err != nil && !errors.Is(err, tc.err) {
				t.Fatalf("query error = %v", err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestMySQLSyncDocumentLockReleaseAfterCancellation(t *testing.T) {
	for _, releaseFails := range []bool{false, true} {
		name := "release-success"
		if releaseFails {
			name = "release-error"
		}
		t.Run(name, func(t *testing.T) {
			pool, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			pool.SetMaxOpenConns(1)
			mock.ExpectQuery(`SELECT GET_LOCK\(\?, \?\)`).WithArgs("sync-document:test", int64(30)).
				WillReturnRows(sqlmock.NewRows([]string{"granted"}).AddRow(1))
			release := mock.ExpectQuery(`SELECT RELEASE_LOCK\(\?\)`).WithArgs("sync-document:test")
			if releaseFails {
				release.WillReturnError(errors.New("release unavailable"))
			} else {
				release.WillReturnRows(sqlmock.NewRows([]string{"released"}).AddRow(1))
			}
			mock.ExpectClose()
			ctx, cancel := context.WithCancel(t.Context())
			unlock, err := acquireMySQLSyncDocumentLock(ctx, pool, "sync-document:test")
			if err != nil {
				cancel()
				t.Fatal(err)
			}
			cancel()
			unlock()
			// The owning callback closes this dedicated pool even on release error.
			if err := pool.Close(); err != nil {
				t.Fatal(err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
