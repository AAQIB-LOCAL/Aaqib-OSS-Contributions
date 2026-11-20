package audit

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

const (
	CurrentSchemaVersion = 2
	IdentityStrategy     = "repository-id + pull-request-number (pull-request ID fallback)"
)

type ContributionRecord struct {
	Repository        string  `json:"repository"`
	Number            int64   `json:"number"`
	Description       string  `json:"description"`
	FirstRecordedIn   string  `json:"firstRecordedIn"`
	LastSeenInSearch  bool    `json:"lastSeenInSearch"`
	RepositoryID      *string `json:"repositoryId"`
	PullRequestID     *string `json:"pullRequestId"`
	PullRequestNodeID *string `json:"pullRequestNodeId"`
}

type RepositoryMetadata struct {
	ID       string `json:"id"`
	FullName string `json:"fullName"`
}

type ContributionLedger struct {
	SchemaVersion                              int                  `json:"schemaVersion"`
	SeededFromCommit                           string               `json:"seededFromCommit,omitempty"`
	AuthoredPullRequestHighWatermark           int64                `json:"authoredPullRequestHighWatermark"`
	AuthoredPullRequestHighWatermarkSource     string               `json:"authoredPullRequestHighWatermarkSource,omitempty"`
	LastObservedPublicAuthoredPullRequestCount int64                `json:"lastObservedPublicAuthoredPullRequestCount"`
	Contributions                              []ContributionRecord `json:"contributions"`
	IdentityStrategy                           string               `json:"identityStrategy"`
}

func NormalizeRepositoryName(repository string) string {
	return strings.ToLower(strings.TrimSpace(repository))
}

func ContributionIdentity(record ContributionRecord) string {
	if record.RepositoryID != nil && strings.TrimSpace(*record.RepositoryID) != "" {
		return fmt.Sprintf("repository:%s#%d", *record.RepositoryID, record.Number)
	}
	if record.PullRequestID != nil && strings.TrimSpace(*record.PullRequestID) != "" {
		return "pull-request:" + *record.PullRequestID
	}
	return fmt.Sprintf("name:%s#%d", NormalizeRepositoryName(record.Repository), record.Number)
}

func MergeContributions(
	existingRecords []ContributionRecord,
	discoveredRecords []ContributionRecord,
	repositoryMetadata map[string]*RepositoryMetadata,
) []ContributionRecord {
	merged := make(map[string]ContributionRecord, len(existingRecords)+len(discoveredRecords))

	add := func(record ContributionRecord, discovered bool) {
		requestedName := NormalizeRepositoryName(record.Repository)
		metadata := repositoryMetadata[requestedName]
		normalized := record
		if metadata != nil {
			if metadata.FullName != "" {
				normalized.Repository = metadata.FullName
			}
			id := metadata.ID
			normalized.RepositoryID = &id
		} else if normalized.RepositoryID == nil {
			normalized.RepositoryID = nil
		}
		if normalized.FirstRecordedIn == "" {
			normalized.FirstRecordedIn = "GitHub Search API"
		}
		if discovered {
			normalized.LastSeenInSearch = true
		}

		key := ContributionIdentity(normalized)
		if previous, ok := merged[key]; ok {
			merged[key] = mergeRecord(previous, normalized)
		} else {
			merged[key] = normalized
		}
	}

	for _, record := range existingRecords {
		add(record, false)
	}
	for _, record := range discoveredRecords {
		add(record, true)
	}

	result := make([]ContributionRecord, 0, len(merged))
	for _, record := range merged {
		result = append(result, record)
	}
	sort.Slice(result, func(i, j int) bool {
		left := NormalizeRepositoryName(result[i].Repository)
		right := NormalizeRepositoryName(result[j].Repository)
		if left != right {
			return left < right
		}
		return result[i].Number < result[j].Number
	})
	return result
}

func mergeRecord(previous, incoming ContributionRecord) ContributionRecord {
	merged := previous
	if incoming.Repository != "" {
		merged.Repository = incoming.Repository
	}
	if incoming.RepositoryID != nil {
		merged.RepositoryID = incoming.RepositoryID
	}
	if merged.RepositoryID == nil {
		merged.RepositoryID = previous.RepositoryID
	}
	merged.Number = incoming.Number
	if previous.Description != "" {
		merged.Description = previous.Description
	} else if incoming.Description != "" {
		merged.Description = incoming.Description
	} else {
		merged.Description = ""
	}
	if previous.FirstRecordedIn != "" {
		merged.FirstRecordedIn = previous.FirstRecordedIn
	} else if incoming.FirstRecordedIn != "" {
		merged.FirstRecordedIn = incoming.FirstRecordedIn
	} else {
		merged.FirstRecordedIn = "GitHub Search API"
	}
	merged.LastSeenInSearch = previous.LastSeenInSearch || incoming.LastSeenInSearch
	if incoming.PullRequestID != nil {
		merged.PullRequestID = incoming.PullRequestID
	}
	if incoming.PullRequestNodeID != nil {
		merged.PullRequestNodeID = incoming.PullRequestNodeID
	}
	return merged
}

func ValidateLedger(ledger ContributionLedger) error {
	for index, record := range ledger.Contributions {
		if record.Repository == "" || strings.Count(record.Repository, "/") != 1 || record.Number < 1 {
			return fmt.Errorf("data/contributions.json contains an invalid contribution record at index %d", index)
		}
	}
	return nil
}

func DecodeLedger(data []byte) (ContributionLedger, error) {
	var ledger ContributionLedger
	if err := json.Unmarshal(data, &ledger); err != nil {
		return ContributionLedger{}, fmt.Errorf("decode contribution ledger: %w", err)
	}
	if ledger.Contributions == nil {
		return ContributionLedger{}, fmt.Errorf("data/contributions.json must contain a contributions array")
	}
	if err := ValidateLedger(ledger); err != nil {
		return ContributionLedger{}, err
	}
	return ledger, nil
}

func EncodeLedger(ledger ContributionLedger) ([]byte, error) {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(ledger); err != nil {
		return nil, fmt.Errorf("encode contribution ledger: %w", err)
	}
	return buffer.Bytes(), nil
}
