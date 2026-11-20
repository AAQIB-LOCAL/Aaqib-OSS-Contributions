package audit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
)

const (
	defaultGitHubAPIBase = "https://api.github.com"
	githubAPIVersion     = "2022-11-28"
	maxSearchAttempts    = 3
	searchPageSize       = 100
	searchResultLimit    = 1000
	repositoryBatchSize  = 10
)

type APIError struct {
	StatusCode int
	Body       string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("GitHub API %d: %s", e.StatusCode, e.Body)
}

type GitHubClient struct {
	Token      string
	APIBaseURL string
	HTTPClient *http.Client
	Logger     *log.Logger
}

type SearchPullRequest struct {
	ID            *int64               `json:"id"`
	NodeID        *string              `json:"node_id"`
	Number        int64                `json:"number"`
	Title         string               `json:"title"`
	HTMLURL       *string              `json:"html_url"`
	RepositoryURL *string              `json:"repository_url"`
	PullRequest   *PullRequestMetadata `json:"pull_request"`
}

type PullRequestMetadata struct {
	MergedAt *string `json:"merged_at"`
}

type GitHubSearchResponse struct {
	TotalCount        *int64              `json:"total_count"`
	IncompleteResults bool                `json:"incomplete_results"`
	Items             []SearchPullRequest `json:"items"`
}

type GitHubRepositoryResponse struct {
	ID       int64  `json:"id"`
	FullName string `json:"full_name"`
}

type AuthoredPullRequestCountResponse struct {
	TotalCount *int64 `json:"total_count"`
}

func NewGitHubClient(token string) *GitHubClient {
	return &GitHubClient{
		Token:      token,
		APIBaseURL: defaultGitHubAPIBase,
		HTTPClient: http.DefaultClient,
		Logger:     log.New(io.Discard, "", 0),
	}
}

func (c *GitHubClient) getJSON(ctx context.Context, path string, params url.Values, authenticated bool, target any) error {
	base := strings.TrimRight(c.APIBaseURL, "/")
	requestURL := base + path
	if len(params) > 0 {
		requestURL += "?" + params.Encode()
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return fmt.Errorf("create GitHub API request: %w", err)
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("X-GitHub-Api-Version", githubAPIVersion)
	if authenticated {
		request.Header.Set("Authorization", "Bearer "+c.Token)
	}

	client := c.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("request GitHub API: %w", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return fmt.Errorf("read GitHub API response: %w", err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return &APIError{StatusCode: response.StatusCode, Body: string(body)}
	}
	if err := json.Unmarshal(body, target); err != nil {
		return fmt.Errorf("decode GitHub API response: %w", err)
	}
	return nil
}

func (c *GitHubClient) FetchAuthoredPullRequestCount(ctx context.Context, author string) (int64, error) {
	params := url.Values{
		"q":        {"author:" + author + " is:pr"},
		"per_page": {"1"},
		"page":     {"1"},
	}
	var result AuthoredPullRequestCountResponse
	if err := c.getJSON(ctx, "/search/issues", params, false, &result); err != nil {
		return 0, fmt.Errorf("public GitHub Search API: %w", err)
	}
	if result.TotalCount == nil || *result.TotalCount < 0 {
		return 0, fmt.Errorf("GitHub Search API did not return a valid authored PR count for %s", author)
	}
	return *result.TotalCount, nil
}

func (c *GitHubClient) SearchPullRequests(ctx context.Context, query string) (int64, []SearchPullRequest, error) {
	var lastError error
	for attempt := 1; attempt <= maxSearchAttempts; attempt++ {
		count, items, err := c.searchPullRequestsOnce(ctx, query)
		if err == nil {
			return count, items, nil
		}
		lastError = err
		if c.Logger != nil {
			c.Logger.Printf("Contribution search attempt %d/%d failed: %v", attempt, maxSearchAttempts, err)
		}
	}
	return 0, nil, fmt.Errorf(
		"unable to retrieve a complete, consistent contribution result set after %d attempts. README.md was not updated. Last error: %v",
		maxSearchAttempts,
		lastError,
	)
}

func (c *GitHubClient) searchPullRequestsOnce(ctx context.Context, query string) (int64, []SearchPullRequest, error) {
	fetchPage := func(page int) (GitHubSearchResponse, error) {
		params := url.Values{
			"q":        {query},
			"per_page": {strconv.Itoa(searchPageSize)},
			"page":     {strconv.Itoa(page)},
		}
		var result GitHubSearchResponse
		if err := c.getJSON(ctx, "/search/issues", params, true, &result); err != nil {
			return GitHubSearchResponse{}, err
		}
		return result, nil
	}

	firstPage, err := fetchPage(1)
	if err != nil {
		return 0, nil, err
	}
	if firstPage.TotalCount == nil || firstPage.Items == nil {
		return 0, nil, errors.New("GitHub Search API returned an incomplete first-page response")
	}
	totalCount := *firstPage.TotalCount
	if totalCount < 0 {
		return 0, nil, errors.New("GitHub Search API returned an invalid result count")
	}
	if firstPage.IncompleteResults {
		return 0, nil, errors.New("GitHub Search API marked the first page as incomplete")
	}
	if totalCount > searchResultLimit {
		return 0, nil, fmt.Errorf("GitHub Search API returned %d results, exceeding its 1,000-result pagination limit; refusing to publish an incomplete audit", totalCount)
	}

	pageCount := int((totalCount + searchPageSize - 1) / searchPageSize)
	results := make([]SearchPullRequest, 0, totalCount)
	results = append(results, firstPage.Items...)
	for page := 2; page <= pageCount; page++ {
		data, err := fetchPage(page)
		if err != nil {
			return 0, nil, err
		}
		if data.IncompleteResults {
			return 0, nil, fmt.Errorf("GitHub Search API marked page %d as incomplete", page)
		}
		if data.TotalCount == nil || data.Items == nil {
			return 0, nil, fmt.Errorf("GitHub Search API returned an incomplete response for page %d", page)
		}
		if *data.TotalCount != totalCount {
			return 0, nil, fmt.Errorf("GitHub Search API result count changed from %d to %d while paging", totalCount, *data.TotalCount)
		}
		results = append(results, data.Items...)
	}

	unique := make([]SearchPullRequest, 0, len(results))
	seen := make(map[string]struct{}, len(results))
	for _, item := range results {
		key := ""
		if item.ID != nil {
			key = strconv.FormatInt(*item.ID, 10)
		} else if item.HTMLURL != nil {
			key = *item.HTMLURL
		} else {
			repositoryURL := ""
			if item.RepositoryURL != nil {
				repositoryURL = *item.RepositoryURL
			}
			key = fmt.Sprintf("%s#%d", repositoryURL, item.Number)
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		unique = append(unique, item)
	}
	if int64(len(unique)) != totalCount {
		return 0, nil, fmt.Errorf("GitHub Search API reported %d results but returned %d unique items across %d pages", totalCount, len(unique), pageCount)
	}
	return totalCount, unique, nil
}

var repositoryURLPattern = regexp.MustCompile(`/repos/([^/]+)/([^/]+)$`)

func RepositoryFromURL(repositoryURL *string) (string, bool) {
	if repositoryURL == nil {
		return "", false
	}
	match := repositoryURLPattern.FindStringSubmatch(*repositoryURL)
	if len(match) != 3 {
		return "", false
	}
	return match[1] + "/" + match[2], true
}

func (c *GitHubClient) ResolveRepositoryMetadata(
	ctx context.Context,
	repositoryNames []string,
	storedRecords []ContributionRecord,
	liveRepositoryNames map[string]struct{},
) (map[string]*RepositoryMetadata, error) {
	metadata := make(map[string]*RepositoryMetadata)
	for _, record := range storedRecords {
		if record.RepositoryID == nil || strings.TrimSpace(*record.RepositoryID) == "" {
			continue
		}
		metadata[NormalizeRepositoryName(record.Repository)] = &RepositoryMetadata{
			ID:       *record.RepositoryID,
			FullName: record.Repository,
		}
	}

	unique := make(map[string]string)
	for _, name := range repositoryNames {
		key := NormalizeRepositoryName(name)
		if _, exists := metadata[key]; exists {
			continue
		}
		unique[key] = name
	}
	unresolved := make([]string, 0, len(unique))
	for _, name := range unique {
		unresolved = append(unresolved, name)
	}
	sort.Slice(unresolved, func(i, j int) bool {
		return strings.ToLower(unresolved[i]) < strings.ToLower(unresolved[j])
	})

	for offset := 0; offset < len(unresolved); offset += repositoryBatchSize {
		end := offset + repositoryBatchSize
		if end > len(unresolved) {
			end = len(unresolved)
		}
		batch := unresolved[offset:end]
		results := make([]repositoryResolution, len(batch))
		var waitGroup sync.WaitGroup
		for index, repository := range batch {
			waitGroup.Add(1)
			go func(index int, repository string) {
				defer waitGroup.Done()
				results[index] = c.resolveOneRepository(ctx, repository, liveRepositoryNames)
			}(index, repository)
		}
		waitGroup.Wait()
		for _, result := range results {
			if result.err != nil {
				return nil, result.err
			}
			metadata[NormalizeRepositoryName(result.requestedName)] = result.metadata
			if result.metadata != nil && result.metadata.FullName != "" {
				metadata[NormalizeRepositoryName(result.metadata.FullName)] = result.metadata
			}
		}
	}
	return metadata, nil
}

type repositoryResolution struct {
	requestedName string
	metadata      *RepositoryMetadata
	err           error
}

func (c *GitHubClient) resolveOneRepository(ctx context.Context, repository string, live map[string]struct{}) repositoryResolution {
	parts := strings.Split(repository, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return repositoryResolution{requestedName: repository, err: fmt.Errorf("invalid repository full name %q", repository)}
	}
	path := "/repos/" + url.PathEscape(parts[0]) + "/" + url.PathEscape(parts[1])
	var result GitHubRepositoryResponse
	err := c.getJSON(ctx, path, nil, true, &result)
	if err != nil {
		var apiError *APIError
		if errors.As(err, &apiError) && apiError.StatusCode == http.StatusNotFound {
			if _, isLive := live[NormalizeRepositoryName(repository)]; !isLive {
				if c.Logger != nil {
					c.Logger.Printf("Keeping historical repository identity by name because it is no longer accessible: %s", repository)
				}
				return repositoryResolution{requestedName: repository, metadata: nil}
			}
		}
		return repositoryResolution{
			requestedName: repository,
			err:           fmt.Errorf("unable to resolve repository identity for %s; refusing to publish a potentially duplicated ledger. %w", repository, err),
		}
	}
	if result.ID <= 0 || result.FullName == "" {
		return repositoryResolution{
			requestedName: repository,
			err:           fmt.Errorf("unable to resolve repository identity for %s; GitHub returned incomplete repository identity metadata", repository),
		}
	}
	return repositoryResolution{
		requestedName: repository,
		metadata:      &RepositoryMetadata{ID: strconv.FormatInt(result.ID, 10), FullName: result.FullName},
	}
}
