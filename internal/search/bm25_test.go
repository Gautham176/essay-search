package search

import (
	"math"
	"testing"
)

// floatEq compares floats with a small tolerance, since exact equality
// on floats is rarely the right check.
func floatEq(a, b float64) bool {
	return math.Abs(a-b) < 1e-6
}

func TestIDF(t *testing.T) {
	cases := []struct {
		name      string
		totalDocs int
		df        int
		// We won't assert exact values — instead we assert ordering and
		// sign properties, which is what we actually care about.
	}{
		{"common term: df = N", 173, 173},   // should be near 0
		{"medium term: df = N/5", 173, 35},  // should be positive, moderate
		{"rare term: df = 2", 173, 2},       // should be positive, large
		{"singleton: df = 1", 173, 1},       // should be largest
	}

	scores := make(map[string]float64)
	for _, c := range cases {
		scores[c.name] = IDF(c.totalDocs, c.df)
		t.Logf("%s: idf=%.4f", c.name, scores[c.name])
	}

	// Sanity checks: rarer terms must have higher IDF.
	if scores["common term: df = N"] >= scores["medium term: df = N/5"] {
		t.Errorf("common term IDF should be less than medium term IDF")
	}
	if scores["medium term: df = N/5"] >= scores["rare term: df = 2"] {
		t.Errorf("medium term IDF should be less than rare term IDF")
	}
	if scores["rare term: df = 2"] >= scores["singleton: df = 1"] {
		t.Errorf("singleton term should have highest IDF")
	}

	// IDF should never be negative with the smoothed formula.
	for name, s := range scores {
		if s < 0 {
			t.Errorf("%s: IDF should be non-negative, got %.4f", name, s)
		}
	}
}

func TestScore(t *testing.T) {
	const avgDocLen = 2000.0
	const idf = 2.0 // pretend IDF for tests

	// Property 1: more occurrences → higher score, but with diminishing returns.
	score1 := Score(1, 2000, avgDocLen, idf)
	score5 := Score(5, 2000, avgDocLen, idf)
	score50 := Score(50, 2000, avgDocLen, idf)
	t.Logf("tf=1: %.4f  tf=5: %.4f  tf=50: %.4f", score1, score5, score50)

	if !(score1 < score5 && score5 < score50) {
		t.Errorf("score should be monotonically increasing in tf")
	}
	// Diminishing returns: gain from 5→50 should be less than gain from 1→5,
	// even though the tf jump is 10x bigger.
	if (score50 - score5) >= (score5 - score1) {
		t.Errorf("expected diminishing returns: tf 5→50 gain (%.4f) should be less than tf 1→5 gain (%.4f)",
			score50-score5, score5-score1)
	}

	// Property 2: longer docs score lower at equal TF (Problem 3 from BM25 intuition).
	shortDoc := Score(10, 1000, avgDocLen, idf)
	avgDoc := Score(10, 2000, avgDocLen, idf)
	longDoc := Score(10, 4000, avgDocLen, idf)
	t.Logf("dl=1000: %.4f  dl=2000: %.4f  dl=4000: %.4f", shortDoc, avgDoc, longDoc)

	if !(shortDoc > avgDoc && avgDoc > longDoc) {
		t.Errorf("shorter docs should score higher at equal tf")
	}

	// Property 3: zero IDF → zero score (common terms contribute nothing).
	if got := Score(10, 2000, avgDocLen, 0); !floatEq(got, 0) {
		t.Errorf("zero IDF should give zero score, got %.4f", got)
	}
}