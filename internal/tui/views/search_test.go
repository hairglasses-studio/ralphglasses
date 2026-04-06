package views

import (
	"testing"

	"github.com/hairglasses-studio/ralphglasses/internal/model"
	"github.com/hairglasses-studio/ralphglasses/internal/session"
	"github.com/hairglasses-studio/ralphglasses/internal/tui/components"
)

func TestSearch_EmptyQuery(t *testing.T) {
	repos := []*model.Repo{{Name: "foo", Path: "/tmp/foo"}}
	results := Search("", repos, nil, nil)
	if len(results) != 0 {
		t.Errorf("expected 0 results for empty query, got %d", len(results))
	}
}

func TestSearch_ExactMatch(t *testing.T) {
	repos := []*model.Repo{
		{Name: "ralphglasses", Path: "/home/user/ralphglasses"},
		{Name: "internal-audit", Path: "/home/user/internal-audit"},
	}
	results := Search("ralphglasses", repos, nil, nil)
	if len(results) == 0 {
		t.Fatal("expected results for exact match")
	}
	if results[0].Name != "ralphglasses" {
		t.Errorf("expected first result to be ralphglasses, got %s", results[0].Name)
	}
	if results[0].Score != 100 {
		t.Errorf("expected score 100 for exact match, got %.0f", results[0].Score)
	}
	if results[0].Type != components.SearchTypeRepo {
		t.Errorf("expected type repo, got %s", results[0].Type)
	}
}

func TestSearch_PrefixMatch(t *testing.T) {
	repos := []*model.Repo{
		{Name: "ralphglasses", Path: "/home/user/ralphglasses"},
		{Name: "internal-audit", Path: "/home/user/internal-audit"},
	}
	results := Search("ralph", repos, nil, nil)
	if len(results) == 0 {
		t.Fatal("expected results for prefix match")
	}
	if results[0].Score != 80 {
		t.Errorf("expected score 80 for prefix match, got %.0f", results[0].Score)
	}
}

func TestSearch_ContainsMatch(t *testing.T) {
	repos := []*model.Repo{
		{Name: "ralphglasses", Path: "/home/user/ralphglasses"},
	}
	results := Search("glass", repos, nil, nil)
	if len(results) == 0 {
		t.Fatal("expected results for contains match")
	}
	if results[0].Score != 60 {
		t.Errorf("expected score 60 for contains match, got %.0f", results[0].Score)
	}
}

func TestSearch_FuzzyMatch(t *testing.T) {
	repos := []*model.Repo{
		{Name: "ralphglasses", Path: "/home/user/ralphglasses"},
	}
	results := Search("rgs", repos, nil, nil)
	if len(results) == 0 {
		t.Fatal("expected results for fuzzy match")
	}
	if results[0].Score != 40 {
		t.Errorf("expected score 40 for fuzzy match, got %.0f", results[0].Score)
	}
}

func TestSearch_NoMatch(t *testing.T) {
	repos := []*model.Repo{
		{Name: "ralphglasses", Path: "/home/user/ralphglasses"},
	}
	results := Search("zzz", repos, nil, nil)
	if len(results) != 0 {
		t.Errorf("expected 0 results for no match, got %d", len(results))
	}
}

func TestSearch_ScoreRanking(t *testing.T) {
	repos := []*model.Repo{
		{Name: "abc", Path: "/tmp/abc"},
		{Name: "abcdef", Path: "/tmp/abcdef"},
		{Name: "xabcy", Path: "/tmp/xabcy"},
	}
	results := Search("abc", repos, nil, nil)
	if len(results) < 3 {
		t.Fatalf("expected at least 3 results, got %d", len(results))
	}
	// Exact match first, prefix second, contains third
	if results[0].Name != "abc" {
		t.Errorf("expected exact match first, got %s", results[0].Name)
	}
	if results[1].Name != "abcdef" {
		t.Errorf("expected prefix match second, got %s", results[1].Name)
	}
	if results[2].Name != "xabcy" {
		t.Errorf("expected contains match third, got %s", results[2].Name)
	}
}

func TestSearch_Sessions(t *testing.T) {
	sessions := []SessionInfo{
		{ID: "sess-123456", RepoName: "myrepo", Provider: "claude", Status: "running", Prompt: "fix bugs"},
	}
	results := Search("myrepo", nil, sessions, nil)
	if len(results) == 0 {
		t.Fatal("expected session results")
	}
	if results[0].Type != components.SearchTypeSession {
		t.Errorf("expected type session, got %s", results[0].Type)
	}
}

func TestSearch_Cycles(t *testing.T) {
	cycles := []*session.CycleRun{
		{ID: "cycle-1", Name: "test-cycle", Phase: session.CycleExecuting, Objective: "improve coverage"},
	}
	results := Search("test-cycle", nil, nil, cycles)
	if len(results) == 0 {
		t.Fatal("expected cycle results")
	}
	if results[0].Type != components.SearchTypeCycle {
		t.Errorf("expected type cycle, got %s", results[0].Type)
	}
}

func TestSearch_SpecialCharacters(t *testing.T) {
	repos := []*model.Repo{
		{Name: "my-repo_v2.0", Path: "/tmp/my-repo_v2.0"},
	}
	// Dashes and underscores should match
	results := Search("my-repo", repos, nil, nil)
	if len(results) == 0 {
		t.Fatal("expected results with special characters")
	}

	// Dots in query
	results = Search("v2.0", repos, nil, nil)
	if len(results) == 0 {
		t.Fatal("expected results matching dot in name")
	}
}

func TestSearch_CaseInsensitive(t *testing.T) {
	repos := []*model.Repo{
		{Name: "RalphGlasses", Path: "/tmp/RalphGlasses"},
	}
	results := Search("ralphglasses", repos, nil, nil)
	if len(results) == 0 {
		t.Fatal("expected case-insensitive match")
	}
}

func TestSearch_MaxResults(t *testing.T) {
	var repos []*model.Repo
	for range 30 {
		repos = append(repos, &model.Repo{
			Name: "repo-match",
			Path: "/tmp/repo-match",
		})
	}
	results := Search("repo", repos, nil, nil)
	if len(results) > 20 {
		t.Errorf("expected max 20 results, got %d", len(results))
	}
}

func TestSearch_MixedTypes(t *testing.T) {
	repos := []*model.Repo{
		{Name: "alpha", Path: "/tmp/alpha"},
	}
	sessions := []SessionInfo{
		{ID: "sess-alpha", RepoName: "alpha", Provider: "claude", Status: "running"},
	}
	cycles := []*session.CycleRun{
		{ID: "cycle-alpha", Name: "alpha-cycle", Phase: session.CycleComplete, Objective: "alpha test"},
	}
	results := Search("alpha", repos, sessions, cycles)
	if len(results) < 3 {
		t.Fatalf("expected at least 3 results across types, got %d", len(results))
	}

	// Verify we have all types
	types := make(map[components.SearchResultType]bool)
	for _, r := range results {
		types[r.Type] = true
	}
	if !types[components.SearchTypeRepo] {
		t.Error("missing repo type in results")
	}
	if !types[components.SearchTypeSession] {
		t.Error("missing session type in results")
	}
	if !types[components.SearchTypeCycle] {
		t.Error("missing cycle type in results")
	}
}

func TestFuzzyMatch(t *testing.T) {
	tests := []struct {
		query, field string
		want         bool
	}{
		{"abc", "aXbXc", true},
		{"abc", "abcdef", true},
		{"xyz", "abc", false},
		{"abc", "ab", false},
		{"", "abc", true},
		{"a", "", false},
	}
	for _, tt := range tests {
		got := fuzzyMatch(tt.query, tt.field)
		if got != tt.want {
			t.Errorf("fuzzyMatch(%q, %q) = %v, want %v", tt.query, tt.field, got, tt.want)
		}
	}
}

func TestScoreField(t *testing.T) {
	tests := []struct {
		query, field string
		want         float64
	}{
		{"abc", "abc", 100},
		{"abc", "abcdef", 80},
		{"def", "abcdef", 60},
		{"adf", "abcdef", 40},
		{"zzz", "abcdef", 0},
		{"", "abc", 0},
		{"abc", "", 0},
	}
	for _, tt := range tests {
		got := scoreField(tt.query, tt.field)
		if got != tt.want {
			t.Errorf("scoreField(%q, %q) = %.0f, want %.0f", tt.query, tt.field, got, tt.want)
		}
	}
}

func TestSearchView_Interface(t *testing.T) {
	var _ View = (*SearchView)(nil)
}
