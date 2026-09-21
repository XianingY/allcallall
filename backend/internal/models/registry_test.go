package models

import (
	"reflect"
	"regexp"
	"testing"
)

// tabler is the GORM interface that pins a model to an explicit table name.
type tabler interface{ TableName() string }

// tableNamePattern matches the lower_snake_case convention used by every table
// in this schema (digits allowed, no leading/trailing/double underscores).
var tableNamePattern = regexp.MustCompile(`^[a-z][a-z0-9]*(_[a-z0-9]+)*$`)

// TestAllModels_RegistryEntriesAreValid guards the schema registry that new
// database bootstrap and AutoMigrate rely on. A bad entry here silently creates
// or migrates the wrong table, so the invariants are asserted explicitly:
//   1. every entry is a pointer to a struct (gorm rejects non-pointers);
//   2. every model pins an explicit, non-empty TableName();
//   3. table names follow the lower_snake_case convention;
//   4. no two models share a table name (a copy/paste accident would make one
//      model overwrite the other's columns).
func TestAllModels_RegistryEntriesAreValid(t *testing.T) {
	seen := make(map[string]string)
	for _, model := range AllModels() {
		value := reflect.ValueOf(model)
		if value.Kind() != reflect.Ptr {
			t.Errorf("AllModels() entry %T must be a pointer to a struct, got %s", model, value.Kind())
			continue
		}
		typ := value.Type().Elem()
		name := typ.Name()
		if typ.Kind() != reflect.Struct {
			t.Errorf("AllModels() entry %T must point at a struct", model)
			continue
		}

		tbl, ok := model.(tabler)
		if !ok {
			t.Errorf("%s does not implement TableName(); add an explicit TableName() so AutoMigrate targets a stable table", name)
			continue
		}
		table := tbl.TableName()
		if table == "" {
			t.Errorf("%s.TableName() returned an empty string", name)
			continue
		}
		if !tableNamePattern.MatchString(table) {
			t.Errorf("%s.TableName() = %q, want lower_snake_case", name, table)
		}
		if other, dup := seen[table]; dup {
			t.Errorf("duplicate table name %q is claimed by both %s and %s", table, other, name)
			continue
		}
		seen[table] = name
	}
}

// TestAllModels_RegistryIsNotEmpty fails loudly if the registry is ever
// emptied by a bad merge, which would silently skip schema bootstrap.
func TestAllModels_RegistryIsNotEmpty(t *testing.T) {
	if got := len(AllModels()); got < 50 {
		t.Fatalf("len(AllModels()) = %d, want at least 50 registered models", got)
	}
}
