import fs from "node:fs";
import process from "node:process";

const token = process.env.GITHUB_TOKEN;
const owner = "AAQIB-LOCAL";
const repo = "Aaqib-OSS-Contributions";
const author = "Aaqibhafeezkhan";
const excludedOwners = new Set(
  (process.env.EXCLUDED_GITHUB_OWNERS || "Aaqibhafeezkhan,AAQIB-LOCAL,AIOps-Capabilities-Hub,buildtheportfolio,stillbrainstorming,hafeez-dev-labs")
    .split(",")
    .map((value) => value.trim().toLowerCase())
    .filter(Boolean),
);

if (!token) {
  throw new Error("GITHUB_TOKEN is required");
}

async function github(path, searchParams) {
  const url = new URL(`https://api.github.com${path}`);
  for (const [key, value] of Object.entries(searchParams || {})) {
    url.searchParams.set(key, value);
  }

  const response = await fetch(url, {
    headers: {
      Accept: "application/vnd.github+json",
      Authorization: `Bearer ${token}`,
      "X-GitHub-Api-Version": "2022-11-28",
    },
  });

  if (!response.ok) {
    const body = await response.text();
    throw new Error(`GitHub API ${response.status}: ${body}`);
  }

  return response.json();
}

async function searchPullRequests(query) {
  const results = [];
  for (let page = 1; ; page += 1) {
    const data = await github("/search/issues", {
      q: query,
      per_page: "100",
      page: String(page),
    });
    results.push(...data.items);
    if (data.items.length < 100 || results.length >= data.total_count) {
      return { totalCount: data.total_count, items: results };
    }
  }
}

function repositoryFromUrl(repositoryUrl) {
  const match = repositoryUrl?.match(/\/repos\/([^/]+)\/([^/]+)$/);
  if (!match) {
    return null;
  }
  return `${match[1]}/${match[2]}`;
}

function markdownLink(repository) {
  return `[${repository}](https://github.com/${repository})`;
}

function escapeMarkdown(value) {
  return String(value ?? "").replaceAll("|", "\\|").replaceAll("\n", " ").trim();
}

function existingDescriptions(readme) {
  const descriptions = new Map();
  const rowPattern = /^\| (\[.*?\]\(https:\/\/github\.com\/([^/]+\/[^)]+)\)) \| (?:\[#(\d+)\]\(https:\/\/github\.com\/[^/]+\/[^/]+\/pull\/\d+\)|#(\d+)) \| (.*?) \| Merged \|$/gm;
  for (const match of readme.matchAll(rowPattern)) {
    const number = match[3] || match[4];
    descriptions.set(match[2] + "#" + number, match[5]);
  }
  return descriptions;
}

function buildTable(items, descriptions) {
  return items
    .map((item) => {
      const repository = repositoryFromUrl(item.repository_url);
      const key = `${repository}#${item.number}`;
      const description = descriptions.get(key) || escapeMarkdown(item.title);
      return `| ${markdownLink(repository)} | [#${item.number}](https://github.com/${repository}/pull/${item.number}) | ${description} | Merged |`;
    })
    .join("\n");
}

const allPullRequests = await searchPullRequests(`author:${author} is:pr`);
const mergedPullRequests = await searchPullRequests(`author:${author} is:pr is:merged`);
const externalMerged = mergedPullRequests.items
  .filter((item) => item.pull_request?.merged_at)
  .filter((item) => {
    const repository = repositoryFromUrl(item.repository_url);
    const repositoryOwner = repository?.split("/")[0]?.toLowerCase();
    return repositoryOwner && !excludedOwners.has(repositoryOwner);
  })
  .sort((a, b) => repositoryFromUrl(a.repository_url).localeCompare(repositoryFromUrl(b.repository_url)) || a.number - b.number);

const readmePath = "README.md";
const existingReadme = fs.readFileSync(readmePath, "utf8");
const descriptions = existingDescriptions(existingReadme);
const repositoryCounts = new Map();
for (const item of externalMerged) {
  const repository = repositoryFromUrl(item.repository_url);
  repositoryCounts.set(repository, (repositoryCounts.get(repository) || 0) + 1);
}

const repositoryBreakdown = [...repositoryCounts.entries()]
  .sort((a, b) => b[1] - a[1] || a[0].localeCompare(b[0]))
  .map(([repository, count]) => `| ${markdownLink(repository)} | ${count} |`)
  .join("\n");

const auditTable = buildTable(externalMerged, descriptions);
const repositoryCount = repositoryCounts.size;
const mergedCount = externalMerged.length;

const readme = `# Aaqib OSS Contributions

A technical record of verified open-source contributions by \`${author}\`, based on upstream pull-request history.

## Contribution Summary

| Metric | Count |
|---|---:|
| Authored GitHub PRs discovered | ${allPullRequests.totalCount} |
| Verified merged upstream OSS PRs | ${mergedCount} |
| External OSS repositories with merged PRs | ${repositoryCount} |

The authored-PR search population includes personal, organization/internal, fork-based, and external repositories. The OSS ledger below counts only merged PRs in repositories outside the configured non-external namespaces.

## Verified Upstream OSS Contributions

| Repository | PR | Technical contribution | Status |
|---|---:|---|---|
${auditTable}

## External OSS Contribution Breakdown

| Repository | Merged PRs |
|---|---:|
${repositoryBreakdown}
| **Total** | **${mergedCount}** |

## Contribution Areas

- **Java / JVM:** JVM maintenance, validation, regression testing, and integration testing
- **JavaScript / TypeScript:** CLI, web, observability, parser, and type-inference changes
- **Databases / Infrastructure:** SQLite and PostgreSQL stores plus Redis/Testcontainers integration testing
- **Testing / Quality:** Property-based fuzzing, contract suites, integration tests, component tests, desktop regression tests, and deterministic unit coverage
- **Security / Reliability:** Authorization boundaries, input validation, concurrent key rotation, duplicate-submission prevention, public-share state handling, and defensive test coverage
- **Developer tooling:** CLI JSON output, quiet/reporting modes, store contracts, and contributor-facing reliability improvements
- **Open-source content:** Kana Dojo vocabulary/content contribution
- **Documentation:** Fair-Code corrections and contributor-list work
- **First contribution:** First Contributions contributor-list entry

## Technical Focus

- **Java / JVM:** API contract enforcement, startup/configuration validation, and regression coverage.
- **Persistence and infrastructure:** SQLite/PostgreSQL storage adapters, Redis-backed integration tests, atomic increments, and TTL handling.
- **JavaScript / TypeScript:** CLI improvements, stable JSON output, parser fuzzing, type inference, and component-level concurrency handling.
- **Security and reliability:** Route-level authorization, safe public-share state handling, concurrent key rotation, and duplicate-submission prevention.
- **Quality engineering:** Property-based fuzzing, contract suites, integration tests, component tests, and desktop regression tests.
- **Developer experience:** CLI quiet/report modes, machine-readable output, reusable store contracts, and contributor-facing fixes.

## Full GitHub Activity Audit

The broader audit currently discovers **${allPullRequests.totalCount} authored GitHub pull requests** for \`${author}\`. The external OSS subset is derived from the merged population after namespace filtering.

### Namespace handling

The automation excludes the configured personal, organizational, portfolio, and internal namespaces from the external OSS ledger. Update the \`EXCLUDED_GITHUB_OWNERS\` environment variable in the workflow when another namespace should be treated as non-external.

### Fork handling

Fork-based branches are not treated as separate external repositories when the merged pull request is submitted to an external upstream repository. The upstream repository and merged PR are the canonical contribution record.

### Deterministic updates

Existing contribution descriptions are preserved from the current README. Newly discovered contributions use their GitHub PR titles until manually refined. Re-running the audit without new contribution data produces no README change.

## Automation

GitHub Actions refreshes this date-free ledger weekly or on manual workflow dispatch.

The workflow:

1. audits authored GitHub PRs through the GitHub API;
2. regenerates date-free contribution rows and technical summaries;
3. commits the README directly to the main branch only when content changes.


## Contribution Counting Rules

Only merged pull requests to external upstream repositories appear in the verified ledger. Open PRs, issue work, fork-only commits, and unmerged changes are not counted as completed contributions.
`;

fs.writeFileSync(readmePath, readme.endsWith("\n") ? readme : `${readme}\n`);
