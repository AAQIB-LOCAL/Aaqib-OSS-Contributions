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

async function fetchAuthoredPullRequestCount() {
  const data = await github("/search/issues", {
    q: `author:${author} is:pr`,
    per_page: "1",
    page: "1",
  });

  if (!Number.isInteger(data.total_count) || data.total_count < 0) {
    throw new Error(`GitHub Search API did not return a valid authored PR count for ${author}`);
  }

  return data.total_count;
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

const authoredPullRequestCount = await fetchAuthoredPullRequestCount();
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

A portfolio of shipped open-source work by \`${author}\` — focused on practical engineering improvements, reliable software, and contributions merged into upstream projects.

**Highlights:** backend and API reliability · developer tooling · test quality · security · TypeScript/JavaScript · Java/JVM

## Recruiter Quick View

**Senior Software Engineer | Full Stack Developer**

I build and improve web applications, backend services, APIs, and developer tooling, with a focus on reliability, clear contracts, and regression-tested changes.

- **Core technologies:** Java, Spring Boot, TypeScript/JavaScript, React, Next.js, REST and GraphQL APIs
- **Engineering strengths:** backend reliability, API validation, concurrency safety, automated testing, persistence, and CLI tooling
- **Profiles:** [LinkedIn](https://www.linkedin.com/in/aaqibhafeezkhan/) · [GitHub](https://github.com/Aaqibhafeezkhan) · [Portfolio](https://aaqibhafeezkhan.github.io/aaqibhafeezkhan.cv/extended-cv/)
- **Review the evidence:** start with the selected merged PRs below, then expand the full contribution ledger for the complete record.


[Featured work](#selected-engineering-work) · [Full contribution ledger](#verified-upstream-oss-contributions) · [Engineering themes](#engineering-themes)

## Contribution Snapshot

| Metric | Count |
|---|---:|
| Publicly discoverable authored GitHub PRs (Search API) | ${authoredPullRequestCount} |
| Verified merged upstream OSS PRs | ${mergedCount} |
| External OSS repositories with merged PRs | ${repositoryCount} |

The authored-PR count uses GitHub's public Search API query author:${author} is:pr and reflects publicly discoverable pull requests. It may differ from the signed-in pulls/authored view because that view can include results not exposed by public search. The OSS ledger below counts only merged PRs in repositories outside the configured non-external namespaces.

## Selected Engineering Work

A few examples that show the kind of problems addressed across upstream projects:

- **Concurrency and security:** [Prevent duplicate environment-share submissions](https://github.com/getsotto/sotto/pull/278) with component-level concurrency protection and regression coverage.
- **Backend reliability:** [Fix concurrent environment key rotation](https://github.com/getsotto/sotto/pull/252) with regression coverage.
- **JVM contracts:** [Enforce non-null API behavior](https://github.com/Sekelenao/Flinkboot/pull/143) and [validate execution parallelism](https://github.com/Sekelenao/Flinkboot/pull/144).
- **Developer experience:** [Add stable JSON output](https://github.com/getsotto/sotto/pull/275) and CLI reporting improvements.
- **Data and persistence:** [Add PostgreSQL-backed storage](https://github.com/Bubblegunn/proactive-gate/pull/25) with atomic increments, TTL handling, and contract coverage.
- **Open-source participation:** [First Contributions contributor entry](https://github.com/firstcontributions/first-contributions/pull/123775).

## Verified Upstream OSS Contributions

<details>
<summary>Browse all ${mergedCount} merged pull requests</summary>

| Repository | PR | Technical contribution | Status |
|---|---:|---|---|
${auditTable}

</details>

## External OSS Contribution Breakdown

<details>
<summary>Browse merged PR counts across ${repositoryCount} external repositories</summary>

| Repository | Merged PRs |
|---|---:|
${repositoryBreakdown}
| **Total** | **${mergedCount}** |

</details>

## Engineering Themes

- **Backend and APIs:** contract enforcement, configuration validation, persistence adapters, and safe state transitions.
- **Reliability and security:** concurrency controls, authorization boundaries, input validation, and regression prevention.
- **Developer tooling:** CLI JSON/report modes, parser and type-inference improvements, and contributor workflows.
- **Quality engineering:** property-based fuzzing, contract suites, integration tests, and deterministic regression coverage.
- **Java/JVM and TypeScript/JavaScript:** contributions across backend services, command-line tools, and web components.

## Audit Methodology

<details>
<summary>Counting rules and automation details</summary>

The broader audit currently discovers **${authoredPullRequestCount} authored GitHub pull requests** for \`${author}\`. This is a public Search API total, not a count of merged contributions. The verified OSS ledger counts merged PRs after filtering configured non-external namespaces.

- **Namespace handling:** The automation excludes configured personal, organizational, portfolio, and internal namespaces. Update EXCLUDED_GITHUB_OWNERS in the workflow when needed.
- **Fork handling:** A merged PR is attributed to the external upstream repository where it lands, not to the contributor's fork.
- **Deterministic updates:** Existing contribution descriptions are preserved; new entries use PR titles until refined. Re-running without new contribution data produces no README change.

</details>

## Automation

GitHub Actions refreshes this date-free ledger daily or on manual workflow dispatch.

The workflow:

1. audits authored GitHub PRs through the GitHub API;
2. regenerates date-free contribution rows and technical summaries;
3. commits the README directly to the main branch only when content changes.


## Contribution Counting Rules

Only merged pull requests to external upstream repositories appear in the verified ledger. Open PRs, issue work, fork-only commits, and unmerged changes are not counted as completed contributions.
`;

fs.writeFileSync(readmePath, readme.endsWith("\n") ? readme : `${readme}\n`);
