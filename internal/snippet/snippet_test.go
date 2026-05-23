package snippet

import (
	"strings"
	"testing"
)

func TestSnippet(t *testing.T) {
	cases := []struct {
		name      string
		body      string
		query     []string // already-stemmed query terms
		wantMarks int      // expected number of <mark> tags
		mustContain []string
	}{
		{
			name:      "single match in long body",
			body:      strings.Repeat("padding ", 50) + "the startup found its niche " + strings.Repeat("padding ", 50),
			query:     []string{"startup"},
			wantMarks: 1,
			mustContain: []string{"<mark>startup</mark>"},
		},
		{
			name:      "multiple matches close together",
			body:      "lisp is great. lisp is powerful. lisp is fun.",
			query:     []string{"lisp"},
			wantMarks: 3,
			mustContain: []string{"<mark>lisp</mark>"},
		},
		{
			name:      "no matches",
			body:      "this body contains none of the query terms whatsoever",
			query:     []string{"xyzzy"},
			wantMarks: 0,
		},
		{
			name:      "short body returns whole thing",
			body:      "lisp is good",
			query:     []string{"lisp"},
			wantMarks: 1,
			mustContain: []string{"<mark>lisp</mark>"},
		},
		{
			name:      "empty query",
			body:      "any body",
			query:     []string{},
			wantMarks: 0,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Snippet(c.body, c.query)
			t.Logf("snippet: %q", got)

			gotMarks := strings.Count(got, "<mark>")
			if gotMarks != c.wantMarks {
				t.Errorf("got %d <mark> tags, want %d", gotMarks, c.wantMarks)
			}
			// Every <mark> should have a matching </mark>.
			if strings.Count(got, "<mark>") != strings.Count(got, "</mark>") {
				t.Errorf("unbalanced mark tags: %q", got)
			}
			for _, want := range c.mustContain {
				if !strings.Contains(got, want) {
					t.Errorf("snippet should contain %q, got %q", want, got)
				}
			}
		})
	}
}