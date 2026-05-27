// Command eval runs a set of labeled queries through every search mode
// and reports precision and rank metrics. Reads queries from a JSON file;
// hits the running server's /search endpoint.

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"
)

type queryCase struct {
	Query    string   `json:"query"`
	Category string   `json:"category"`
	Ideal    []string `json:"ideal"`
}

type apiResult struct {
	Title string `json:"title"`
}

type apiResponse struct {
	Results []apiResult `json:"results"`
}

func main() {
	queriesPath := flag.String("queries", "evals/queries.json", "path to queries JSON file")
	server := flag.String("server", "http://localhost:8080", "search server base URL")
	flag.Parse()

	cases, err := loadCases(*queriesPath)
	if err != nil {
		log.Fatalf("load queries: %v", err)
	}
	log.Printf("loaded %d query cases", len(cases))

	modes := []string{"keyword", "semantic", "hybrid"}

	// results[mode][caseIndex] = rank of first relevant result, or 0 if none in top 10
	rankOfHit := make(map[string][]int)
	for _, m := range modes {
		rankOfHit[m] = make([]int, len(cases))
	}
	// p5Hits[mode][caseIndex] = count of relevant results in top 5
	p5Hits := make(map[string][]int)
	for _, m := range modes {
		p5Hits[m] = make([]int, len(cases))
	}

	for i, c := range cases {
		for _, m := range modes {
			titles, err := runQuery(*server, c.Query, m, 10)
			if err != nil {
				log.Printf("[%d] %s/%s: %v", i, c.Query, m, err)
				continue
			}
			rankOfHit[m][i] = firstRelevantRank(titles, c.Ideal)
			p5Hits[m][i] = countRelevantInTop(titles, c.Ideal, 5)
		}
	}

	// Print overall table.
	fmt.Println()
	fmt.Println("OVERALL METRICS")
	fmt.Println(strings.Repeat("-", 50))
	fmt.Printf("%-10s %8s %8s %8s\n", "mode", "P@1", "P@5", "MRR")
	for _, m := range modes {
		p1 := computeP1(rankOfHit[m])
		p5 := computeP5(p5Hits[m])
		mrr := computeMRR(rankOfHit[m])
		fmt.Printf("%-10s %8.3f %8.3f %8.3f\n", m, p1, p5, mrr)
	}

	// Print per-category breakdown.
	categories := uniqueCategories(cases)
	fmt.Println()
	fmt.Println("BY CATEGORY (P@1)")
	fmt.Println(strings.Repeat("-", 50))
	fmt.Printf("%-10s", "mode")
	for _, cat := range categories {
		fmt.Printf(" %10s", cat)
	}
	fmt.Println()
	for _, m := range modes {
		fmt.Printf("%-10s", m)
		for _, cat := range categories {
			indices := indicesForCategory(cases, cat)
			subset := pickIndices(rankOfHit[m], indices)
			fmt.Printf(" %10.3f", computeP1(subset))
		}
		fmt.Println()
	}

	// Print per-query details so a human can sanity-check.
	fmt.Println()
	fmt.Println("PER-QUERY DETAIL (rank of first relevant result; 0 = not in top 10)")
	fmt.Println(strings.Repeat("-", 80))
	fmt.Printf("%-45s %8s %10s %8s\n", "query", "keyword", "semantic", "hybrid")
	for i, c := range cases {
		fmt.Printf("%-45s %8d %10d %8d\n",
			truncateForTable(c.Query, 45),
			rankOfHit["keyword"][i],
			rankOfHit["semantic"][i],
			rankOfHit["hybrid"][i],
		)
	}
}

func loadCases(path string) ([]queryCase, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var cases []queryCase
	if err := json.NewDecoder(f).Decode(&cases); err != nil {
		return nil, err
	}
	return cases, nil
}

// runQuery hits the server and returns the result titles in rank order.
func runQuery(server, query, mode string, k int) ([]string, error) {
	u, err := url.Parse(server + "/search")
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("q", query)
	q.Set("mode", mode)
	q.Set("k", fmt.Sprintf("%d", k))
	u.RawQuery = q.Encode()

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(u.String())
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		buf, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, string(buf))
	}
	var body apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}
	titles := make([]string, len(body.Results))
	for i, r := range body.Results {
		titles[i] = r.Title
	}
	return titles, nil
}

// firstRelevantRank returns 1-indexed rank of the first relevant title,
// or 0 if no relevant title appears in titles.
func firstRelevantRank(titles []string, ideal []string) int {
	idealSet := make(map[string]bool, len(ideal))
	for _, t := range ideal {
		idealSet[t] = true
	}
	for i, t := range titles {
		if idealSet[t] {
			return i + 1
		}
	}
	return 0
}

func countRelevantInTop(titles []string, ideal []string, n int) int {
	idealSet := make(map[string]bool, len(ideal))
	for _, t := range ideal {
		idealSet[t] = true
	}
	if n > len(titles) {
		n = len(titles)
	}
	count := 0
	for i := 0; i < n; i++ {
		if idealSet[titles[i]] {
			count++
		}
	}
	return count
}

// computeP1 returns fraction of cases where rank == 1.
func computeP1(ranks []int) float64 {
	if len(ranks) == 0 {
		return 0
	}
	hits := 0
	for _, r := range ranks {
		if r == 1 {
			hits++
		}
	}
	return float64(hits) / float64(len(ranks))
}

// computeP5 returns the average number of relevant results in top 5,
// divided by 5. This is fraction of top-5 slots filled with relevant docs.
func computeP5(hitsPerCase []int) float64 {
	if len(hitsPerCase) == 0 {
		return 0
	}
	total := 0
	for _, h := range hitsPerCase {
		total += h
	}
	return float64(total) / (5.0 * float64(len(hitsPerCase)))
}

// computeMRR returns mean reciprocal rank. 1/rank if rank > 0, else 0.
func computeMRR(ranks []int) float64 {
	if len(ranks) == 0 {
		return 0
	}
	sum := 0.0
	for _, r := range ranks {
		if r > 0 {
			sum += 1.0 / float64(r)
		}
	}
	return sum / float64(len(ranks))
}

func uniqueCategories(cases []queryCase) []string {
	seen := make(map[string]bool)
	var out []string
	for _, c := range cases {
		if c.Category != "" && !seen[c.Category] {
			seen[c.Category] = true
			out = append(out, c.Category)
		}
	}
	sort.Strings(out)
	return out
}

func indicesForCategory(cases []queryCase, cat string) []int {
	var out []int
	for i, c := range cases {
		if c.Category == cat {
			out = append(out, i)
		}
	}
	return out
}

func pickIndices(xs []int, indices []int) []int {
	out := make([]int, len(indices))
	for i, idx := range indices {
		out[i] = xs[idx]
	}
	return out
}

func truncateForTable(s string, n int) string {
	if len(s) <= n {
		return s + strings.Repeat(" ", n-len(s))
	}
	return s[:n-3] + "..."
}