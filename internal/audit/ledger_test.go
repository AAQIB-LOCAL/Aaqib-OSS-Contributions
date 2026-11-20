package audit

import (
	"reflect"
	"strings"
	"testing"
)

func stringPointer(value string) *string { return &value }

func TestMergeContributionsCoalescesRepositoryRenames(t *testing.T) {
	metadata := map[string]*RepositoryMetadata{
		"old-owner/project": {ID: "42", FullName: "new-owner/project"},
		"new-owner/project": {ID: "42", FullName: "new-owner/project"},
	}
	merged := MergeContributions([]ContributionRecord{
		{Repository: "old-owner/project", Number: 7, Description: "Curated title", FirstRecordedIn: "seed"},
	}, []ContributionRecord{
		{Repository: "new-owner/project", Number: 7, Description: "Raw title", FirstRecordedIn: "GitHub Search API", LastSeenInSearch: true},
	}, metadata)
	if len(merged) != 1 || merged[0].Repository != "new-owner/project" || merged[0].RepositoryID == nil || *merged[0].RepositoryID != "42" {
		t.Fatalf("rename did not coalesce into canonical identity: %#v", merged)
	}
	if merged[0].Description != "Curated title" || merged[0].FirstRecordedIn != "seed" || !merged[0].LastSeenInSearch {
		t.Fatalf("historical fields were not preserved: %#v", merged[0])
	}
}

func TestMergeContributionsKeepsSamePRNumberAcrossDifferentRepositories(t *testing.T) {
	records := []ContributionRecord{
		{Repository: "one/project", Number: 12, RepositoryID: stringPointer("101")},
		{Repository: "two/project", Number: 12, RepositoryID: stringPointer("202")},
	}
	merged := MergeContributions(records, nil, nil)
	if len(merged) != 2 || ContributionIdentity(merged[0]) == ContributionIdentity(merged[1]) {
		t.Fatalf("unrelated repositories were merged: %#v", merged)
	}
}

func TestMergeContributionsRetainsUnavailableHistoricalRepository(t *testing.T) {
	records := []ContributionRecord{{Repository: "deleted/repo", Number: 3, Description: "Retained", FirstRecordedIn: "seed"}}
	merged := MergeContributions(records, nil, map[string]*RepositoryMetadata{"deleted/repo": nil})
	if len(merged) != 1 || merged[0].Repository != "deleted/repo" || merged[0].Description != "Retained" {
		t.Fatalf("historical record was not retained: %#v", merged)
	}
}

func TestMergeContributionsIsIdempotent(t *testing.T) {
	metadata := map[string]*RepositoryMetadata{"owner/repo": {ID: "42", FullName: "owner/repo"}}
	existing := []ContributionRecord{{Repository: "owner/repo", Number: 1, RepositoryID: stringPointer("42"), Description: "Curated"}}
	discovered := []ContributionRecord{{Repository: "owner/repo", Number: 1, Description: "Title", LastSeenInSearch: true}}
	first := MergeContributions(existing, discovered, metadata)
	second := MergeContributions(first, discovered, metadata)
	if !reflect.DeepEqual(first, second) { t.Fatalf("merge is not idempotent: %#v != %#v", first, second) }
}

func TestContributionIdentityFallsBackToPRIDThenRepositoryName(t *testing.T) {
	withID := ContributionRecord{Repository: "owner/repo", Number: 2, PullRequestID: stringPointer("77")}
	if got := ContributionIdentity(withID); got != "pull-request:77" { t.Fatalf("PR ID fallback = %q", got) }
	withoutIDs := ContributionRecord{Repository: " Owner/Repo ", Number: 2}
	if got := ContributionIdentity(withoutIDs); got != "name:owner/repo#2" { t.Fatalf("repository fallback = %q", got) }
}

func TestDecodeLedgerRejectsMalformedAndInvalidRecords(t *testing.T) {
	for _, body := range []string{"{\"schemaVersion\":2}", "{\"contributions\":[{\"repository\":\"owner/repo/extra\",\"number\":1}]}", "not-json"} {
		if _, err := DecodeLedger([]byte(body)); err == nil { t.Fatalf("invalid ledger accepted: %s", body) }
	}
	if err := ValidateLedger(ContributionLedger{Contributions: []ContributionRecord{{Repository: "owner/repo", Number: 0}}}); err == nil {
		t.Fatal("zero PR number should be invalid")
	}
}

func TestEncodeLedgerDoesNotEscapeHTML(t *testing.T) {
	encoded, err := EncodeLedger(ContributionLedger{Contributions: []ContributionRecord{{Repository: "owner/repo", Number: 1, Description: "A & B < C"}}})
	if err != nil { t.Fatal(err) }
	if strings.Contains(string(encoded), "\\u0026") || strings.Contains(string(encoded), "\\u003c") {
		t.Fatalf("JSON encoder HTML-escaped content: %s", encoded)
	}
	if !strings.HasSuffix(string(encoded), "\n") { t.Fatal("encoded ledger must end with a newline") }
}
