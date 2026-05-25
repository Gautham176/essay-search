package chunk

import (
	"strings"
	"testing"
)

func TestChunk(t *testing.T) {
	t.Run("empty input returns nil", func(t *testing.T) {
		if got := Chunk("", 100, 10); got != nil {
			t.Errorf("got %v, want nil", got)
		}
	})

	t.Run("short doc returns one chunk", func(t *testing.T) {
		text := "this is a short document"
		got := Chunk(text, 100, 10)
		if len(got) != 1 {
			t.Fatalf("got %d chunks, want 1", len(got))
		}
		if got[0] != text {
			t.Errorf("got %q, want %q", got[0], text)
		}
	})

	t.Run("exact chunk size returns one chunk", func(t *testing.T) {
		words := make([]string, 100)
		for i := range words {
			words[i] = "word"
		}
		text := strings.Join(words, " ")
		got := Chunk(text, 100, 10)
		if len(got) != 1 {
			t.Errorf("got %d chunks, want 1", len(got))
		}
	})

	t.Run("two chunks with overlap", func(t *testing.T) {
		// 150 words, chunkSize=100, overlap=20.
		// step = 80, so chunks should be: [0-100), [80-150)
		words := make([]string, 150)
		for i := range words {
			words[i] = wordAt(i)
		}
		text := strings.Join(words, " ")
		got := Chunk(text, 100, 20)

		if len(got) != 2 {
			t.Fatalf("got %d chunks, want 2", len(got))
		}

		// First chunk: 100 words starting at "w0".
		first := strings.Fields(got[0])
		if len(first) != 100 || first[0] != "w0" || first[99] != "w99" {
			t.Errorf("first chunk has wrong range: starts %q, ends %q, len %d",
				first[0], first[len(first)-1], len(first))
		}

		// Second chunk: 70 words (150 - 80) starting at "w80".
		second := strings.Fields(got[1])
		if len(second) != 70 || second[0] != "w80" || second[69] != "w149" {
			t.Errorf("second chunk has wrong range: starts %q, ends %q, len %d",
				second[0], second[len(second)-1], len(second))
		}
	})

	t.Run("many chunks", func(t *testing.T) {
		// 1000 words, chunkSize=300, overlap=50, step=250.
		// Chunks: [0-300), [250-550), [500-800), [750-1000)  → 4 chunks
		words := make([]string, 1000)
		for i := range words {
			words[i] = wordAt(i)
		}
		text := strings.Join(words, " ")
		got := Chunk(text, 300, 50)

		if len(got) != 4 {
			t.Errorf("got %d chunks, want 4", len(got))
		}
		// Last chunk should end with w999.
		last := strings.Fields(got[len(got)-1])
		if last[len(last)-1] != "w999" {
			t.Errorf("last chunk doesn't end at w999: ends %q", last[len(last)-1])
		}
	})

	t.Run("no tiny trailing chunk", func(t *testing.T) {
		words := make([]string, 305)
		for i := range words {
			words[i] = wordAt(i)
		}
		text := strings.Join(words, " ")
		got := Chunk(text, 300, 50)

		if len(got) != 2 {
			t.Errorf("got %d chunks, want 2", len(got))
		}
	})
}

// wordAt produces unique word names so we can verify chunk boundaries.
func wordAt(i int) string {
	return "w" + itoa(i)
}

func itoa(n int) string {
	// Tiny int-to-string to avoid pulling in strconv.
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}