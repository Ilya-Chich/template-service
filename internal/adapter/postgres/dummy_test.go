package postgres

import (
	"context"
	"errors"
	"fmt"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"http-api-template/internal/core/domain"
	"testing"
)

const dummyID = "550e8400-e29b-41d4-a716-446655440000"
const dummyName = "Example"
const readIDQuery = "SELECT .* FROM \"dummies\" WHERE id = \\$1"
const updateQuery = "UPDATE \"dummies\" SET \"note\"=\\$1 WHERE id = \\$2"
const deleteQuery = "DELETE FROM \"dummies\" WHERE id = \\$1"

var errDatabase = errors.New("database unavailable")

func repositoryForTest(t *testing.T) (*Dummy, sqlmock.Sqlmock) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{
		DisableAutomaticPing: true,
		Logger:               logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Error(err)
		}
		mock.ExpectClose()
		if err := Close(db); err != nil {
			t.Error(err)
		}
	})
	return NewDummy(db), mock
}
func TestRead(t *testing.T) {
	for _, tc := range []struct {
		name    string
		byName  bool
		missing bool
		dbErr   error
		wantErr error
	}{
		{"ID", false, false, nil, nil},
		{"name", true, false, nil, nil},
		{"missing ID", false, true, nil, domain.ErrNotFound},
		{"missing name", true, true, nil, domain.ErrNotFound},
		{"ID failure", false, false, errDatabase, errDatabase},
		{"name failure", true, false, errDatabase, errDatabase},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo, mock := repositoryForTest(t)
			query, arg := readIDQuery, dummyID
			if tc.byName {
				query, arg = "SELECT .* FROM \"dummies\" WHERE name = \\$1", dummyName
			}
			expect := mock.ExpectQuery(query).WithArgs(arg)
			if tc.dbErr != nil {
				expect.WillReturnError(tc.dbErr)
			} else {
				rows := sqlmock.NewRows([]string{"id", "name", "note"})
				if !tc.missing {
					rows.AddRow(dummyID, dummyName, "")
				}
				expect.WillReturnRows(rows)
			}
			var got domain.Dummy
			var err error
			if tc.byName {
				got, err = repo.GetByName(context.Background(), arg)
			} else {
				got, err = repo.Get(context.Background(), arg)
			}
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("error=%v", err)
			}
			if err == nil && got != (domain.Dummy{ID: dummyID, Name: dummyName}) {
				t.Fatalf("unexpected domain value: %v", got)
			}
		})
	}
}
func TestInsert(t *testing.T) {
	for _, tc := range []struct {
		name    string
		rows    int64
		dbErr   error
		wantErr error
	}{
		{"insert", 1, nil, nil},
		{"replay ID or name", 0, nil, nil},
		{"failure", 0, errDatabase, errDatabase},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo, mock := repositoryForTest(t)
			mock.ExpectBegin()
			expect := mock.ExpectExec("INSERT INTO \"dummies\" .* ON CONFLICT DO NOTHING").WithArgs(dummyID, dummyName, "")
			if tc.dbErr != nil {
				expect.WillReturnError(tc.dbErr)
				mock.ExpectRollback()
			} else {
				expect.WillReturnResult(sqlmock.NewResult(0, tc.rows))
				mock.ExpectCommit()
			}
			err := repo.Insert(context.Background(), domain.Dummy{ID: dummyID, Name: dummyName})
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("error=%v", err)
			}
		})
	}
}
func TestMutations(t *testing.T) {
	empty := ""
	for _, tc := range []struct {
		name    string
		remove  bool
		rows    int64
		dbErr   error
		wantErr error
	}{
		{"clear note", false, 1, nil, nil},
		{"missing update", false, 0, nil, nil},
		{"update conflict", false, 0, &pgconn.PgError{Code: "23505"}, domain.ErrConflict},
		{"update invalid name", false, 0, &pgconn.PgError{Code: "23514"}, domain.ErrInvalid},
		{"update failure", false, 0, errDatabase, errDatabase},
		{"delete", true, 1, nil, nil},
		{"missing delete", true, 0, nil, nil},
		{"delete failure", true, 0, errDatabase, errDatabase},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo, mock := repositoryForTest(t)
			mock.ExpectBegin()
			var expect *sqlmock.ExpectedExec
			if tc.remove {
				expect = mock.ExpectExec(deleteQuery).WithArgs(dummyID)
			} else {
				expect = mock.ExpectExec(updateQuery).WithArgs(empty, dummyID)
			}
			if tc.dbErr != nil {
				expect.WillReturnError(tc.dbErr)
				mock.ExpectRollback()
			} else {
				expect.WillReturnResult(sqlmock.NewResult(0, tc.rows))
				mock.ExpectCommit()
			}
			var err error
			if tc.remove {
				err = repo.Delete(context.Background(), dummyID)
			} else {
				err = repo.Update(context.Background(), dummyID, domain.Patch{Note: &empty})
			}
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("error=%v", err)
			}
		})
	}
}
func TestUpdateFields(t *testing.T) {
	repo, mock := repositoryForTest(t)
	name, note := " Renamed ", ""
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE \"dummies\" SET \"name\"=\\$1,\"note\"=\\$2 WHERE id = \\$3").
		WithArgs(name, note, dummyID).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if err := repo.Update(context.Background(), dummyID, domain.Patch{Name: &name, Note: &note}); err != nil {
		t.Fatal(err)
	}
	if err := repo.Update(context.Background(), dummyID, domain.Patch{}); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("empty patch error=%v", err)
	}
}
func TestMapError(t *testing.T) {
	unique := &pgconn.PgError{Code: "23505"}
	for _, tc := range []struct {
		name  string
		input error
		want  error
	}{
		{"success", nil, nil},
		{"missing", fmt.Errorf("query: %w", gorm.ErrRecordNotFound), domain.ErrNotFound},
		{"unique", fmt.Errorf("query: %w", unique), domain.ErrConflict},
		{"check constraint", &pgconn.PgError{Code: "23514"}, domain.ErrInvalid},
		{"unavailable", errDatabase, errDatabase},
		{"other database error", &pgconn.PgError{Code: "08006"}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := mapError(tc.input)
			if tc.name == "other database error" {
				if !errors.Is(got, tc.input) {
					t.Fatal("unexpected database error was not preserved")
				}
			} else if !errors.Is(got, tc.want) {
				t.Fatalf("error=%v", got)
			}
		})
	}
	if !errors.Is(mapError(unique), unique) {
		t.Fatal("original driver error was not preserved")
	}
}
func TestCanceledContext(t *testing.T) {
	repo, _ := repositoryForTest(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := repo.Get(ctx, dummyID); !errors.Is(err, context.Canceled) {
		t.Fatalf("error=%v", err)
	}
}
func TestOpenCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	db, err := Open(ctx, "postgres://example:example@localhost:5432/example?sslmode=disable")
	if db != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("expected canceled open, got database=%v error=%v", db, err)
	}
}
