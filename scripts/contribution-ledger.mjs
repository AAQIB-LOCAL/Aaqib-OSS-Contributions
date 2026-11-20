export function normalizeRepositoryName(repository) {
  return String(repository ?? "").trim().toLowerCase();
}

export function contributionIdentity(record) {
  const number = Number(record.number);
  if (record.repositoryId !== undefined && record.repositoryId !== null && String(record.repositoryId).trim() !== "") {
    return `repository:${String(record.repositoryId)}#${number}`;
  }
  if (record.pullRequestId !== undefined && record.pullRequestId !== null && String(record.pullRequestId).trim() !== "") {
    return `pull-request:${String(record.pullRequestId)}`;
  }
  return `name:${normalizeRepositoryName(record.repository)}#${number}`;
}

function mergeRecord(previous, incoming) {
  return {
    ...previous,
    ...incoming,
    repository: incoming.repository || previous.repository,
    repositoryId: incoming.repositoryId ?? previous.repositoryId ?? null,
    number: incoming.number,
    description: previous.description || incoming.description || "",
    firstRecordedIn: previous.firstRecordedIn || incoming.firstRecordedIn || "GitHub Search API",
    lastSeenInSearch: Boolean(previous.lastSeenInSearch || incoming.lastSeenInSearch),
    pullRequestId: incoming.pullRequestId ?? previous.pullRequestId ?? null,
    pullRequestNodeId: incoming.pullRequestNodeId ?? previous.pullRequestNodeId ?? null,
  };
}

/**
 * Canonicalize repository aliases and merge contribution records without
 * confusing equal PR numbers from unrelated repositories.
 *
 * repositoryMetadata is keyed by the lower-cased requested full name and holds
 * { id, fullName } returned by GET /repos/{owner}/{repo}. GitHub redirects old
 * repository names to their canonical repository, so aliases share one ID.
 */
export function mergeContributions(existingRecords, discoveredRecords = [], repositoryMetadata = new Map()) {
  const merged = new Map();

  function add(record, discovered) {
    const requestedName = normalizeRepositoryName(record.repository);
    const metadata = repositoryMetadata.get(requestedName) ?? null;
    const normalized = {
      ...record,
      repository: metadata?.fullName || record.repository,
      repositoryId: metadata?.id ?? record.repositoryId ?? null,
      firstRecordedIn: record.firstRecordedIn || "GitHub Search API",
      pullRequestId: record.pullRequestId ?? null,
      pullRequestNodeId: record.pullRequestNodeId ?? null,
      lastSeenInSearch: discovered ? true : Boolean(record.lastSeenInSearch),
    };
    const key = contributionIdentity(normalized);
    const previous = merged.get(key);
    merged.set(key, previous ? mergeRecord(previous, normalized) : normalized);
  }

  for (const record of existingRecords) add(record, false);
  for (const record of discoveredRecords) add(record, true);

  return [...merged.values()].sort((a, b) =>
    normalizeRepositoryName(a.repository).localeCompare(normalizeRepositoryName(b.repository)) ||
    a.number - b.number,
  );
}
