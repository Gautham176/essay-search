package tokenize

import (
	"reflect"
	"testing"
)

func TestTokenize(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{
			name: "basic lowercase and split",
			in:   "Go is Fast",
			want: []string{"go", "fast"}, // "is" is a stopword
		},
		{
			name: "stemming",
			in:   "running quickly",
			want: []string{"run", "quickli"},
		},
		{
			name: "stopwords removed",
			in:   "the cat and the dog",
			want: []string{"cat", "dog"},
		},
		{
			name: "apostrophe contraction",
			in:   "don't stop",
			want: []string{"dont", "stop"},
		},
		{
			name: "punctuation stripped",
			in:   "hello, world!",
			want: []string{"hello", "world"},
		},
		{
			name: "short tokens dropped",
			in:   "a big cat",
			want: []string{"big", "cat"}, // "a" is length 1
		},
		{
			name: "mixed punctuation numbers and empty strings",
			in:   "Ready... 1, 2, 3 --- 'action'!!!",
			want: []string{"readi", "action"}, // 1, 2, 3 drop out because length < 2, '---' is skipped entirely
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Tokenize(c.in)
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("Tokenize(%q)\n  got:  %v\n  want: %v", c.in, got, c.want)
			}
		})
	}
}