package audit

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const defaultExcludedOwners = "Aaqibhafeezkhan,AAQIB-LOCAL,AIOps-Capabilities-Hub,buildtheportfolio,stillbrainstorming,hafeez-dev-labs"

type Config struct {
	Client         *GitHubClient
	Author         string
	ReadmePath     string
	LedgerPath     string
	ExcludedOwners map[string]struct{}
	Logger         *log.Logger
}

type repositoryCount struct {
	Repository string
	Count      int64
}

func ParseExcludedOwners(value string) map[string]struct{} {
	if value == "" {
		value = defaultExcludedOwners
	}
	owners := make(map[string]struct{})
	for _, owner := range strings.Split(value, ",") {
		owner = strings.ToLower(strings.TrimSpace(owner))
		if owner != "" {
			owners[owner] = struct{}{}
		}
	}
	return owners
}

func Run(ctx context.Context, config Config) error {
	if config.Client == nil {
		return fmt.Errorf("GitHub client is required")
	}
	if config.Author == "" {
		return fmt.Errorf("GitHub author is required")
	}
	if config.ReadmePath == "" {
		config.ReadmePath = "README.md"
	}
	if config.LedgerPath == "" {
		config.LedgerPath = filepath.Join("data", "contributions.json")
	}
	if config.ExcludedOwners == nil {
		config.ExcludedOwners = ParseExcludedOwners("")
	}

	observedAuthoredCount, err := config.Client.FetchAuthoredPullRequestCount(ctx, config.Author)
	if err != nil {
		return err
	}
	if config.Logger != nil {
		config.Logger.Printf("Public authored PR count from unauthenticated Search API: %d", observedAuthoredCount)
	}

	_, mergedPullRequests, err := config.Client.SearchPullRequests(ctx, "author:"+config.Author+" is:pr is:merged")
	if err != nil {
		return err
	}

	existingReadmeBytes, err := os.ReadFile(config.ReadmePath)
	if err != nil {
		return fmt.Errorf("read README template: %w", err)
	}
	ledgerBytes, err := os.ReadFile(config.LedgerPath)
	if err != nil {
		return fmt.Errorf("read contribution ledger: %w", err)
	}
	storedLedger, err := DecodeLedger(ledgerBytes)
	if err != nil {
		return err
	}

	descriptions := ExistingDescriptions(string(existingReadmeBytes))
	discoveredRepositories := make([]string, 0, len(mergedPullRequests))
	liveRepositoryNames := make(map[string]struct{})
	for _, item := range mergedPullRequests {
		repository, ok := RepositoryFromURL(item.RepositoryURL)
		if !ok || isExcludedOwner(repository, config.ExcludedOwners) {
			continue
		}
		discoveredRepositories = append(discoveredRepositories, repository)
		liveRepositoryNames[NormalizeRepositoryName(repository)] = struct{}{}
	}

	repositoryNames := make([]string, 0, len(storedLedger.Contributions)+len(discoveredRepositories))
	for _, record := range storedLedger.Contributions {
		repositoryNames = append(repositoryNames, record.Repository)
	}
	repositoryNames = append(repositoryNames, discoveredRepositories...)
	repositoryMetadata, err := config.Client.ResolveRepositoryMetadata(ctx, repositoryNames, storedLedger.Contributions, liveRepositoryNames)
	if err != nil {
		return err
	}

	resolvedHistorical := make([]ContributionRecord, 0, len(storedLedger.Contributions))
	for _, record := range storedLedger.Contributions {
		resolved := repositoryMetadata[NormalizeRepositoryName(record.Repository)]
		updated := record
		updated.LastSeenInSearch = false
		if resolved != nil {
			if resolved.FullName != "" {
				updated.Repository = resolved.FullName
			}
			id := resolved.ID
			updated.RepositoryID = &id
		} else if updated.RepositoryID == nil {
			updated.RepositoryID = nil
		}
		resolvedHistorical = append(resolvedHistorical, updated)
	}

	discoveredContributions := make([]ContributionRecord, 0, len(mergedPullRequests))
	for _, item := range mergedPullRequests {
		if item.PullRequest == nil || item.PullRequest.MergedAt == nil || *item.PullRequest.MergedAt == "" {
			continue
		}
		requestedRepository, ok := RepositoryFromURL(item.RepositoryURL)
		if !ok || isExcludedOwner(requestedRepository, config.ExcludedOwners) {
			continue
		}
		resolved := repositoryMetadata[NormalizeRepositoryName(requestedRepository)]
		repository := requestedRepository
		var repositoryID *string
		if resolved != nil {
			if resolved.FullName != "" {
				repository = resolved.FullName
			}
			id := resolved.ID
			repositoryID = &id
		}
		if isExcludedOwner(repository, config.ExcludedOwners) {
			continue
		}

		key := strings.ToLower(repository) + "#" + strconv.FormatInt(item.Number, 10)
		description, found := descriptions[key]
		if !found {
			description = EscapeMarkdown(item.Title)
		}
		var pullRequestID *string
		if item.ID != nil {
			id := strconv.FormatInt(*item.ID, 10)
			pullRequestID = &id
		}
		discoveredContributions = append(discoveredContributions, ContributionRecord{
			Repository:        repository,
			Number:            item.Number,
			Description:       description,
			FirstRecordedIn:   "GitHub Search API",
			LastSeenInSearch:  true,
			RepositoryID:      repositoryID,
			PullRequestID:     pullRequestID,
			PullRequestNodeID: item.NodeID,
		})
	}

	externalContributions := MergeContributions(resolvedHistorical, discoveredContributions, repositoryMetadata)
	authoredPullRequestCount := storedLedger.AuthoredPullRequestHighWatermark
	if observedAuthoredCount > authoredPullRequestCount {
		authoredPullRequestCount = observedAuthoredCount
	}

	breakdown := buildRepositoryBreakdown(externalContributions)
	repositoryCounts := len(breakdown)

	readme, err := RenderReadme(
		string(existingReadmeBytes),
		int64(len(externalContributions)),
		int64(repositoryCounts),
		authoredPullRequestCount,
		"",
		"",
	)
	if err != nil {
		return err
	}

	updatedLedger := storedLedger
	updatedLedger.SchemaVersion = CurrentSchemaVersion
	updatedLedger.IdentityStrategy = IdentityStrategy
	updatedLedger.AuthoredPullRequestHighWatermark = authoredPullRequestCount
	updatedLedger.LastObservedPublicAuthoredPullRequestCount = observedAuthoredCount
	updatedLedger.Contributions = externalContributions
	encodedLedger, err := EncodeLedger(updatedLedger)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(config.LedgerPath), 0o755); err != nil {
		return fmt.Errorf("create ledger directory: %w", err)
	}
	if err := os.WriteFile(config.LedgerPath, encodedLedger, 0o644); err != nil {
		return fmt.Errorf("write contribution ledger: %w", err)
	}
	if err := os.WriteFile(config.ReadmePath, []byte(readme), 0o644); err != nil {
		return fmt.Errorf("write README: %w", err)
	}
	if config.Logger != nil {
		config.Logger.Printf("Updated contribution audit: %d merged PRs across %d external repositories", len(externalContributions), repositoryCounts)
	}
	return nil
}


// buildRepositoryBreakdown aggregates contributions by normalized repository
// name and persists each incremented map value before materializing the sorted
// breakdown.
func buildRepositoryBreakdown(items []ContributionRecord) []repositoryCount {
	counts := make(map[string]repositoryCount)
	for _, item := range items {
		key := strings.ToLower(item.Repository)
		count, exists := counts[key]
		if exists {
			count.Count++
			counts[key] = count
		} else {
			counts[key] = repositoryCount{Repository: item.Repository, Count: 1}
		}
	}

	breakdown := make([]repositoryCount, 0, len(counts))
	for _, item := range counts {
		breakdown = append(breakdown, item)
	}
	sort.Slice(breakdown, func(i, j int) bool {
		if breakdown[i].Count != breakdown[j].Count {
			return breakdown[i].Count > breakdown[j].Count
		}
		left := strings.ToLower(breakdown[i].Repository)
		right := strings.ToLower(breakdown[j].Repository)
		return left < right
	})
	return breakdown
}

func isExcludedOwner(repository string, excluded map[string]struct{}) bool {
	parts := strings.Split(repository, "/")
	if len(parts) != 2 {
		return false
	}
	_, found := excluded[strings.ToLower(parts[0])]
	return found
}
