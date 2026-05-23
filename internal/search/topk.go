package search

import "sort"

// topKByScore returns the up to k doc IDs with the highest scores,
// sorted by score descending. Ties broken by doc ID for determinism.
func topKByScore(scores map[int]float64, k int) []int {
	type pair struct {
		id    int
		score float64
	}
	pairs := make([]pair, 0, len(scores))
	for id, s := range scores {
		pairs = append(pairs, pair{id, s})
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].score != pairs[j].score {
			return pairs[i].score > pairs[j].score
		}
		return pairs[i].id < pairs[j].id
	})
	if k > len(pairs) {
		k = len(pairs)
	}
	out := make([]int, k)
	for i := 0; i < k; i++ {
		out[i] = pairs[i].id
	}
	return out
}