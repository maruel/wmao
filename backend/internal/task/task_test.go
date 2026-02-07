package task

import "testing"

func TestSlugify(t *testing.T) {
	t.Run("LowerCase", func(t *testing.T) {
		got := slugify("fix the auth bug")
		if got != "fix-the-auth-bug" {
			t.Errorf("got %q, want %q", got, "fix-the-auth-bug")
		}
	})
	t.Run("SpecialChars", func(t *testing.T) {
		got := slugify("Add pagination to /api/users")
		if got != "add-pagination-to-api-users" {
			t.Errorf("got %q, want %q", got, "add-pagination-to-api-users")
		}
	})
	t.Run("UpperCase", func(t *testing.T) {
		got := slugify("UPPER CASE")
		if got != "upper-case" {
			t.Errorf("got %q, want %q", got, "upper-case")
		}
	})
	t.Run("Truncation", func(t *testing.T) {
		got := slugify("a " + string(make([]byte, 100)))
		if len(got) > 40 {
			t.Errorf("len = %d, want <= 40", len(got))
		}
	})
}
