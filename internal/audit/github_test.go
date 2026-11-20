package audit

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
)

func TestSearchPullRequestsPaginatesAndDeduplicates(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" { t.Fatalf("missing bearer token") }
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		calls.Add(1)
		items := make([]SearchPullRequest, 0)
		if page == 1 {
			for i := int64(1); i <= 100; i++ { id := i; items = append(items, SearchPullRequest{ID: &id, Number: i}) }
		} else {
			id := int64(101)
			items = append(items, SearchPullRequest{ID: &id, Number: 101})
		}
		_ = json.NewEncoder(w).Encode(GitHubSearchResponse{TotalCount: int64Pointer(101), Items: items})
	}))
	defer server.Close()
	client := NewGitHubClient("test-token")
	client.APIBaseURL = server.URL
	count, items, err := client.SearchPullRequests(context.Background(), "author:test is:pr")
	if err != nil { t.Fatal(err) }
	if count != 101 || len(items) != 101 || calls.Load() != 2 { t.Fatalf("count=%d items=%d calls=%d", count, len(items), calls.Load()) }
}

func TestSearchPullRequestsRetriesIncompleteResultsThreeTimes(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_ = json.NewEncoder(w).Encode(GitHubSearchResponse{TotalCount: int64Pointer(0), IncompleteResults: true, Items: []SearchPullRequest{}})
	}))
	defer server.Close()
	client := NewGitHubClient("test-token")
	client.APIBaseURL = server.URL
	_, _, err := client.SearchPullRequests(context.Background(), "author:test is:pr")
	if err == nil || calls.Load() != maxSearchAttempts { t.Fatalf("err=%v calls=%d", err, calls.Load()) }
}

func TestSearchPullRequestsRejectsMoreThanOneThousandResults(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_ = json.NewEncoder(w).Encode(GitHubSearchResponse{TotalCount: int64Pointer(1001), Items: []SearchPullRequest{}})
	}))
	defer server.Close()
	client := NewGitHubClient("test-token")
	client.APIBaseURL = server.URL
	_, _, err := client.SearchPullRequests(context.Background(), "author:test is:pr")
	if err == nil || calls.Load() != maxSearchAttempts { t.Fatalf("err=%v calls=%d", err, calls.Load()) }
}

func TestHistoricalRepository404IsRetainedButLive404Fails(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()
	client := NewGitHubClient("test-token")
	client.APIBaseURL = server.URL
	historical, err := client.ResolveRepositoryMetadata(context.Background(), []string{"deleted/repo"}, nil, map[string]struct{}{})
	if err != nil || historical["deleted/repo"] != nil { t.Fatalf("historical 404 not retained: metadata=%v err=%v", historical, err) }
	_, err = client.ResolveRepositoryMetadata(context.Background(), []string{"new/repo"}, nil, map[string]struct{}{"new/repo": {}})
	if err == nil { t.Fatal("live repository 404 should fail") }
}

func TestAuthoredPullRequestCountUsesUnauthenticatedSearch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" { t.Fatal("auth header must not be sent for public count") }
		if r.URL.Query().Get("q") != "author:Aaqibhafeezkhan is:pr" || r.URL.Query().Get("per_page") != "1" || r.URL.Query().Get("page") != "1" {
			t.Fatalf("unexpected query: %s", r.URL.RawQuery)
		}
		_, _ = w.Write([]byte("{\"total_count\":27}"))
	}))
	defer server.Close()
	client := NewGitHubClient("test-token")
	client.APIBaseURL = server.URL
	count, err := client.FetchAuthoredPullRequestCount(context.Background(), "Aaqibhafeezkhan")
	if err != nil || count != 27 { t.Fatalf("count=%d err=%v", count, err) }
}

func int64Pointer(value int64) *int64 { return &value }
