package audit

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var contributionRowPattern = regexp.MustCompile(`^\| \[.*?\]\(https://github\.com/([^/]+/[^)]+)\) \| (\[#([0-9]+)\]\(https://github\.com/[^/]+/[^/]+/pull/[0-9]+\)|#([0-9]+)) \| (.*?) \| Merged \|$`)

func ExistingDescriptions(readme string) map[string]string {
	descriptions := make(map[string]string)
	for _, line := range strings.Split(readme, "\n") {
		match := contributionRowPattern.FindStringSubmatch(line)
		if len(match) != 6 {
			continue
		}
		repository := match[1]
		number := match[3]
		if number == "" {
			number = match[4]
		}
		descriptions[repository+"#"+number] = match[5]
	}
	return descriptions
}

func EscapeMarkdown(value string) string {
	value = strings.ReplaceAll(value, "|", "\\|")
	value = strings.ReplaceAll(value, "\n", " ")
	return strings.TrimSpace(value)
}

func MarkdownLink(repository string) string {
	return fmt.Sprintf("[%s](https://github.com/%s)", repository, repository)
}

func BuildContributionTable(items []ContributionRecord) string {
	rows := make([]string, 0, len(items))
	for _, item := range items {
		rows = append(rows, fmt.Sprintf("| %s | [#%d](https://github.com/%s/pull/%d) | %s | Merged |", MarkdownLink(item.Repository), item.Number, item.Repository, item.Number, item.Description))
	}
	return strings.Join(rows, "\n")
}

func RenderReadme(template string, mergedCount, repositoryCount, authoredPullRequestCount int64, auditTable, repositoryBreakdown string) (string, error) {
	var err error
	replacements := []struct {
		pattern string
		value   string
	}{
		{`(<td align="center"><strong>)[^<]+(</strong><br/>Merged upstream PRs</td>)`, fmt.Sprintf("${1}%d${2}", mergedCount)},
		{`(<td align="center"><strong>)[^<]+(</strong><br/>External repositories</td>)`, fmt.Sprintf("${1}%d${2}", repositoryCount)},
		{`(<td align="center"><strong>)[^<]+(</strong><br/>Public authored PRs \(high-water mark\)</td>)`, fmt.Sprintf("${1}%d${2}", authoredPullRequestCount)},
		{`Browse all [0-9,]+ merged pull requests`, "Browse all " + strconv.FormatInt(mergedCount, 10) + " merged pull requests"},
		{`Browse merged PR counts across [0-9,]+ external repositories`, "Browse merged PR counts across " + strconv.FormatInt(repositoryCount, 10) + " external repositories"},
		{`broader audit currently discovers \*\*[0-9,]+ authored GitHub pull requests\*\*`, fmt.Sprintf("broader audit currently discovers **%d authored GitHub pull requests**", authoredPullRequestCount)},
	}
	for index, replacement := range replacements {
		pattern, compileErr := regexp.Compile(replacement.pattern)
		if compileErr != nil {
			return "", compileErr
		}
		if !pattern.MatchString(template) {
			// The two "Browse" labels belong to the legacy table-heavy README.
			// The compact dashboard README intentionally omits those labels.
			if index == 3 || index == 4 {
				continue
			}
			return "", fmt.Errorf("README template is missing expected pattern %q", replacement.pattern)
		}
		template = pattern.ReplaceAllString(template, replacement.value)
	}

	auditHeader := "| Repository | PR | Technical contribution | Status |\n|---|---:|---|---|\n"
	if strings.Contains(template, auditHeader) {
		template, err = replaceContentBetween(
			template,
			auditHeader,
			"\n\n</details>",
			"The complete verified contribution ledger is available in the [interactive dashboard](https://aaqib-local.github.io/Aaqib-OSS-Contributions/).",
		)
		if err != nil {
			return "", fmt.Errorf("replace contribution table: %w", err)
		}
		template = strings.Replace(template, auditHeader, "", 1)
	}

	breakdownHeader := "| Repository | Merged PRs |\n|---|---:|\n"
	if strings.Contains(template, breakdownHeader) {
		repositorySummary := fmt.Sprintf(
			"Repository-level totals are available in the [interactive dashboard](https://aaqib-local.github.io/Aaqib-OSS-Contributions/). **%d merged upstream PRs** across **%d external repositories**.",
			mergedCount,
			repositoryCount,
		)
		template, err = replaceContentBetween(template, breakdownHeader, "\n\n</details>", repositorySummary)
		if err != nil {
			return "", fmt.Errorf("replace repository breakdown: %w", err)
		}
		template = strings.Replace(template, breakdownHeader, "", 1)
	}

	if strings.Contains(template, "Repository-level totals are available in the [interactive dashboard]") {
		summary := fmt.Sprintf(
			"Repository-level totals are available in the [interactive dashboard](https://aaqib-local.github.io/Aaqib-OSS-Contributions/). **%d merged upstream PRs** across **%d external repositories**.",
			mergedCount,
			repositoryCount,
		)
		template, err = replaceLineContaining(template, "Repository-level totals are available", summary)
		if err != nil {
			return "", fmt.Errorf("update repository summary: %w", err)
		}
	}

	if !strings.HasSuffix(template, "\n") {
		template += "\n"
	}
	return template, nil
}

func replaceFirstRegex(input, expression, replacement string) (string, error) {
	pattern, err := regexp.Compile(expression)
	if err != nil {
		return "", err
	}
	if !pattern.MatchString(input) {
		return "", fmt.Errorf("README template is missing expected pattern %q", expression)
	}
	return pattern.ReplaceAllString(input, replacement), nil
}

func replaceLineContaining(input, marker, replacement string) (string, error) {
	start := strings.Index(input, marker)
	if start < 0 {
		return "", fmt.Errorf("README template is missing line marker %q", marker)
	}

	lineStart := strings.LastIndex(input[:start], "\n") + 1
	lineEnd := strings.Index(input[start:], "\n")
	if lineEnd < 0 {
		lineEnd = len(input)
	} else {
		lineEnd += start
	}

	return input[:lineStart] + replacement + input[lineEnd:], nil
}

func replaceContentBetween(input, startMarker, endMarker, replacement string) (string, error) {
	start := strings.Index(input, startMarker)
	if start < 0 {
		return "", fmt.Errorf("README template is missing section header %q", startMarker)
	}
	contentStart := start + len(startMarker)
	end := strings.Index(input[contentStart:], endMarker)
	if end < 0 {
		return "", fmt.Errorf("README template is missing section ending %q", endMarker)
	}
	end += contentStart
	return input[:contentStart] + replacement + input[end:], nil
}
