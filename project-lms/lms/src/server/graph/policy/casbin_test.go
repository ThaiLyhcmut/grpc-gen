package policy

import "testing"

func TestExtractID(t *testing.T) {
	type withStr struct{ ID string }
	type withPtr struct{ ID *string }
	type noID struct{ Name string }
	s := "42"

	cases := []struct {
		name string
		in   any
		want string
	}{
		{"nil", nil, ""},
		{"string id via pointer", &withStr{ID: "7"}, "7"},
		{"string id value struct", withStr{ID: "8"}, "8"},
		{"pointer id set", &withPtr{ID: &s}, "42"},
		{"pointer id nil", &withPtr{ID: nil}, ""},
		{"no ID field", &noID{Name: "x"}, ""},
		{"typed nil pointer", (*withStr)(nil), ""},
	}
	for _, c := range cases {
		if got := extractID(c.in); got != c.want {
			t.Errorf("%s: extractID = %q, want %q", c.name, got, c.want)
		}
	}
}
