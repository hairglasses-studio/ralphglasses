package store

import (
	"context"
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open in-memory sqlite: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestNewMigrator_SortsByVersion(t *testing.T) {
	db := openTestDB(t)
	migs := []Migration{
		{Version: 3, Name: "third", Up: "SELECT 1"},
		{Version: 1, Name: "first", Up: "SELECT 1"},
		{Version: 2, Name: "second", Up: "SELECT 1"},
	}
	m := NewMigrator(db, migs)
	if len(m.migrations) != 3 {
		t.Fatalf("expected 3 migrations, got %d", len(m.migrations))
	}
	for i, want := range []int{1, 2, 3} {
		if m.migrations[i].Version != want {
			t.Errorf("migration[%d].Version = %d, want %d", i, m.migrations[i].Version, want)
		}
	}
}

func TestMigrator_Applied_EmptyDB(t *testing.T) {
	db := openTestDB(t)
	m := NewMigrator(db, nil)

	applied, err := m.Applied(context.Background())
	if err != nil {
		t.Fatalf("Applied() error: %v", err)
	}
	if len(applied) != 0 {
		t.Errorf("Applied() on empty DB = %v, want empty", applied)
	}
}

func TestMigrator_Up_AppliesAll(t *testing.T) {
	db := openTestDB(t)
	migs := []Migration{
		{Version: 1, Name: "create_foo", Up: "CREATE TABLE foo (id INTEGER PRIMARY KEY)"},
		{Version: 2, Name: "create_bar", Up: "CREATE TABLE bar (id INTEGER PRIMARY KEY)"},
	}
	m := NewMigrator(db, migs)

	if err := m.Up(context.Background()); err != nil {
		t.Fatalf("Up() error: %v", err)
	}

	applied, err := m.Applied(context.Background())
	if err != nil {
		t.Fatalf("Applied() error: %v", err)
	}
	if !applied[1] || !applied[2] {
		t.Errorf("expected versions 1 and 2 applied, got %v", applied)
	}
}

func TestMigrator_Up_Idempotent(t *testing.T) {
	db := openTestDB(t)
	migs := []Migration{
		{Version: 1, Name: "create_foo", Up: "CREATE TABLE foo (id INTEGER PRIMARY KEY)"},
	}
	m := NewMigrator(db, migs)

	// Apply twice.
	if err := m.Up(context.Background()); err != nil {
		t.Fatalf("first Up() error: %v", err)
	}
	if err := m.Up(context.Background()); err != nil {
		t.Fatalf("second Up() error: %v", err)
	}

	applied, _ := m.Applied(context.Background())
	if !applied[1] {
		t.Error("version 1 should still be applied")
	}
}

func TestMigrator_Down_RollsBack(t *testing.T) {
	db := openTestDB(t)
	migs := []Migration{
		{Version: 1, Name: "create_foo", Up: "CREATE TABLE foo (id INTEGER PRIMARY KEY)", Down: "DROP TABLE foo"},
		{Version: 2, Name: "create_bar", Up: "CREATE TABLE bar (id INTEGER PRIMARY KEY)", Down: "DROP TABLE bar"},
	}
	m := NewMigrator(db, migs)

	if err := m.Up(context.Background()); err != nil {
		t.Fatalf("Up() error: %v", err)
	}

	// Roll back once — should roll back version 2.
	rolled, err := m.Down(context.Background())
	if err != nil {
		t.Fatalf("Down() error: %v", err)
	}
	if rolled != 2 {
		t.Errorf("Down() rolled back v%d, want v2", rolled)
	}

	applied, _ := m.Applied(context.Background())
	if applied[2] {
		t.Error("version 2 should not be applied after rollback")
	}
	if !applied[1] {
		t.Error("version 1 should still be applied")
	}
}

func TestMigrator_Down_NothingToRollBack(t *testing.T) {
	db := openTestDB(t)
	m := NewMigrator(db, nil)

	rolled, err := m.Down(context.Background())
	if err != nil {
		t.Fatalf("Down() on empty: %v", err)
	}
	if rolled != 0 {
		t.Errorf("Down() on empty = %d, want 0", rolled)
	}
}

func TestMigrator_Down_NoDownSQL(t *testing.T) {
	db := openTestDB(t)
	migs := []Migration{
		{Version: 1, Name: "irreversible", Up: "CREATE TABLE x (id INTEGER)"},
		// No Down SQL.
	}
	m := NewMigrator(db, migs)

	if err := m.Up(context.Background()); err != nil {
		t.Fatalf("Up() error: %v", err)
	}

	_, err := m.Down(context.Background())
	if err == nil {
		t.Error("Down() with no Down SQL should return error")
	}
}
