package pagination

import (
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// This package had no tests at all while 18+ list endpoints depend on it, so
// a change to the clamp values would have been caught only in production.

func TestNormalizeClampsToTheAllowedRange(t *testing.T) {
	cases := []struct {
		name       string
		in         Page
		wantLimit  int
		wantOffset int
	}{
		{"zero limit takes the default", Page{Limit: 0, Offset: 0}, DefaultLimit, 0},
		{"negative limit takes the default", Page{Limit: -5, Offset: 0}, DefaultLimit, 0},
		{"oversized limit is capped", Page{Limit: MaxLimit + 1, Offset: 0}, MaxLimit, 0},
		{"limit just below the cap is kept", Page{Limit: MaxLimit - 1, Offset: 0}, MaxLimit - 1, 0},
		{"negative offset becomes zero", Page{Limit: 10, Offset: -20}, 10, 0},
		{"oversized offset is capped", Page{Limit: 10, Offset: MaxOffset + 1}, 10, MaxOffset},
		{"in-range values pass through", Page{Limit: 25, Offset: 100}, 25, 100},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.in.Normalize()
			if got.Limit != tc.wantLimit {
				t.Fatalf("limit = %d, want %d", got.Limit, tc.wantLimit)
			}
			if got.Offset != tc.wantOffset {
				t.Fatalf("offset = %d, want %d", got.Offset, tc.wantOffset)
			}
		})
	}
}

func openTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.Exec("CREATE TABLE items (id integer primary key, name text)").Error; err != nil {
		t.Fatalf("create table: %v", err)
	}
	for i := 1; i <= 25; i++ {
		if err := db.Exec("INSERT INTO items (id, name) VALUES (?, ?)", i, "item").Error; err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	return db
}

// Scope is the whole point of the package: it must normalise before applying,
// so a client cannot bypass the cap by calling Scope directly.
func TestScopeAppliesNormalisedLimitAndOffset(t *testing.T) {
	db := openTestDB(t)

	var page []struct{ ID int }
	if err := db.Table("items").Scopes(Page{Limit: 10, Offset: 5}.Scope).Find(&page).Error; err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(page) != 10 {
		t.Fatalf("returned %d rows, want 10", len(page))
	}
	if page[0].ID != 6 {
		t.Fatalf("first row id = %d, want 6 (offset 5 over ids starting at 1)", page[0].ID)
	}
}

func TestScopeClampsAnOversizedLimit(t *testing.T) {
	db := openTestDB(t)

	var page []struct{ ID int }
	if err := db.Table("items").Scopes(Page{Limit: MaxLimit * 10}.Scope).Find(&page).Error; err != nil {
		t.Fatalf("query: %v", err)
	}
	// Only 25 rows exist, so this proves no error rather than the cap itself;
	// the cap is asserted directly in the Normalize test above.
	if len(page) != 25 {
		t.Fatalf("returned %d rows, want all 25", len(page))
	}
}

func TestNewResultReportsHasMore(t *testing.T) {
	cases := []struct {
		name     string
		total    int64
		page     Page
		wantMore bool
	}{
		{"more pages remain", 100, Page{Limit: 10, Offset: 0}, true},
		{"exactly the last page", 20, Page{Limit: 10, Offset: 10}, false},
		{"past the end", 20, Page{Limit: 10, Offset: 30}, false},
		{"empty result set", 0, Page{Limit: 10, Offset: 0}, false},
		{"single partial page", 3, Page{Limit: 10, Offset: 0}, false},
		{"limit zero is defaulted before comparing", 60, Page{Limit: 0, Offset: 0}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := NewResult([]string{"x"}, tc.total, tc.page)
			if result.HasMore != tc.wantMore {
				t.Fatalf("has_more = %v, want %v (total=%d page=%+v)", result.HasMore, tc.wantMore, tc.total, tc.page)
			}
			// The reported limit must be the normalised one, not what was asked.
			if result.Limit != tc.page.Normalize().Limit {
				t.Fatalf("reported limit = %d, want normalised %d", result.Limit, tc.page.Normalize().Limit)
			}
		})
	}
}
