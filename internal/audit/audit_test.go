package audit

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunPreservesCuratedDescriptionAndHighWatermark(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/search/issues" && strings.HasPrefix(r.URL.Query().Get("q"), "author:") && r.Header.Get("Authorization") == "":
			_, _ = w.Write([]byte(`{"total_count":18}`))
		case r.URL.Path == "/search/issues" && strings.Contains(r.URL.Query().Get("q"), "is:merged"):
			if r.Header.Get("Authorization") != "Bearer test-token" {
				http.Error(w, "missing auth", http.StatusUnauthorized)
				return
			}
			_, _ = w.Write([]byte(`{"total_count":1,"incomplete_results":false,"items":[{"id":900,"node_id":"PR_node","number":4,"title":"New raw title","repository_url":"https://api.github.com/repos/external/repo","pull_request":{"merged_at":"2026-01-01T00:00:00Z"}}]}`))
		case r.URL.Path == "/repos/external/repo":
			_, _ = w.Write([]byte(`{"id":99,"full_name":"external/repo"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	root := t.TempDir()
	readmePath := filepath.Join(root, "README.md")
	ledgerPath := filepath.Join(root, "data", "contributions.json")
	readme := `# Demo
<table>
<tr><td align="center"><strong>1</strong><br/>Merged upstream PRs</td><td align="center"><strong>1</strong><br/>External repositories</td><td align="center"><strong>20</strong><br/>Public authored PRs (high-water mark)</td></tr>
</table>
Browse all 1 merged pull requests
Browse merged PR counts across 1 external repositories
The broader audit currently discovers **20 authored GitHub pull requests** for the author.

| Repository | PR | Technical contribution | Status |
|---|---:|---|---|
| [external/repo](https://github.com/external/repo) | [#4](https://github.com/external/repo/pull/4) | Curated description | Merged |

</details>

| Repository | Merged PRs |
|---|---:|
| [external/repo](https://github.com/external/repo) | 1 |
| **Total** | **1** |
`
	if err := os.WriteFile(readmePath, []byte(readme), 0o644); err != nil {
		t.Fatal(err)
	}
	ledger := ContributionLedger{
		SchemaVersion:                              2,
		SeededFromCommit:                           "seed-sha",
		AuthoredPullRequestHighWatermark:           20,
		AuthoredPullRequestHighWatermarkSource:     "Public GitHub Search API",
		LastObservedPublicAuthoredPullRequestCount: 20,
		Contributions: []ContributionRecord{{
			Repository: "old/repo", Number: 1, Description: "Historical contribution", FirstRecordedIn: "seed", LastSeenInSearch: true,
			RepositoryID: stringPointer("11"), PullRequestID: stringPointer("110"), PullRequestNodeID: stringPointer("PR_old"),
		}},
		IdentityStrategy: IdentityStrategy,
	}
	encoded, err := EncodeLedger(ledger)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(ledgerPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ledgerPath, encoded, 0o644); err != nil {
		t.Fatal(err)
	}

	client := NewGitHubClient("test-token")
	client.APIBaseURL = server.URL
	client.Logger = log.New(os.Stderr, "test: ", 0)
	if err := Run(context.Background(), Config{
		Client: client, Author: "Aaqibhafeezkhan", ReadmePath: readmePath, LedgerPath: ledgerPath,
		ExcludedOwners: ParseExcludedOwners("internal"), Logger: log.New(os.Stdout, "", 0),
	}); err != nil {
		t.Fatal(err)
	}

	updatedLedgerBytes, err := os.ReadFile(ledgerPath)
	if err != nil {
		t.Fatal(err)
	}
	var updated ContributionLedger
	if err := json.Unmarshal(updatedLedgerBytes, &updated); err != nil {
		t.Fatal(err)
	}
	if len(updated.Contributions) != 2 || updated.AuthoredPullRequestHighWatermark != 20 || updated.LastObservedPublicAuthoredPullRequestCount != 18 {
		t.Fatalf("unexpected ledger snapshot: %#v", updated)
	}
	foundCurated := false
	for _, record := range updated.Contributions {
		if record.Repository == "external/repo" && record.Number == 4 {
			foundCurated = record.Description == "Curated description" && record.RepositoryID != nil && *record.RepositoryID == "99"
		}
	}
	if !foundCurated {
		t.Fatalf("new contribution did not preserve its curated description and repository identity: %#v", updated.Contributions)
	}
	updatedReadme, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(updatedReadme), "Historical contribution") || !strings.Contains(string(updatedReadme), "Curated description") || !strings.Contains(string(updatedReadme), "**20 authored GitHub pull requests**") {
		t.Fatalf("generated README is missing expected retained content:\n%s", updatedReadme)
	}
}

func TestRunRejectsMissingConfiguration(t *testing.T) {
	if err := Run(context.Background(), Config{}); err == nil {
		t.Fatal("missing client should fail")
	}
	if err := Run(context.Background(), Config{Client: NewGitHubClient("token")}); err == nil {
		t.Fatal("missing author should fail")
	}
}

func TestRunDoesNotTouchFilesWhenGitHubSearchFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/search/issues" && r.Header.Get("Authorization") == "" {
			_, _ = w.Write([]byte(`{"total_count":9}`))
			return
		}
		http.Error(w, "search unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()
	root := t.TempDir()
	readmePath := filepath.Join(root, "README.md")
	ledgerPath := filepath.Join(root, "data", "contributions.json")
	readme := []byte("unchanged README")
	ledger := []byte(`{"contributions":[]}`)
	if err := os.WriteFile(readmePath, readme, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(ledgerPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ledgerPath, ledger, 0o644); err != nil {
		t.Fatal(err)
	}
	client := NewGitHubClient("token")
	client.APIBaseURL = server.URL
	if err := Run(context.Background(), Config{Client: client, Author: "author", ReadmePath: readmePath, LedgerPath: ledgerPath}); err == nil {
		t.Fatal("GitHub search failure should abort the audit")
	}
	readmeAfter, _ := os.ReadFile(readmePath)
	ledgerAfter, _ := os.ReadFile(ledgerPath)
	if string(readmeAfter) != string(readme) || string(ledgerAfter) != string(ledger) {
		t.Fatal("audit modified files after an API failure")
	}
}


func TestBuildRepositoryBreakdownAggregatesAndSortsCounts(t *testing.T) {
	items := []ContributionRecord{
		{Repository: "owner-b/repo", Number: 1},
		{Repository: "owner-a/repo", Number: 1},
		{Repository: "owner-b/repo", Number: 2},
		{Repository: "owner-c/repo", Number: 1},
		{Repository: "owner-a/repo", Number: 2},
		{Repository: "owner-a/repo", Number: 3},
	}
	breakdown := buildRepositoryBreakdown(items)
	if len(breakdown) != 3 {
		t.Fatalf("got %d repository rows, want 3: %#v", len(breakdown), breakdown)
	}
	want := []repositoryCount{
		{Repository: "owner-a/repo", Count: 3},
		{Repository: "owner-b/repo", Count: 2},
		{Repository: "owner-c/repo", Count: 1},
	}
	for index := range want {
		if breakdown[index] != want[index] {
			t.Fatalf("breakdown[%d] = %#v, want %#v", index, breakdown[index], want[index])
		}
	}
	var total int64
	for _, item := range breakdown {
		total += item.Count
	}
	if total != int64(len(items)) {
		t.Fatalf("breakdown sum = %d, want %d", total, len(items))
	}
}

func TestBuildRepositoryBreakdownSortsEqualCountsByRepository(t *testing.T) {
	items := []ContributionRecord{
		{Repository: "owner-c/repo", Number: 1},
		{Repository: "owner-a/repo", Number: 1},
		{Repository: "owner-b/repo", Number: 1},
	}
	breakdown := buildRepositoryBreakdown(items)
	want := []string{"owner-a/repo", "owner-b/repo", "owner-c/repo"}
	for index, repository := range want {
		if breakdown[index].Repository != repository {
			t.Fatalf("breakdown[%d] = %q, want %q", index, breakdown[index].Repository, repository)
		}
	}
}
