import test from "node:test";
import assert from "node:assert/strict";
import { contributionIdentity, mergeContributions } from "../scripts/contribution-ledger.mjs";

const aliases = new Map([
  ["old-owner/project", { id: "42", fullName: "new-owner/project" }],
  ["new-owner/project", { id: "42", fullName: "new-owner/project" }],
]);

test("coalesces historical records after an upstream repository rename", () => {
  const result = mergeContributions([
    { repository: "old-owner/project", number: 7, description: "Original description", firstRecordedIn: "seed", lastSeenInSearch: false },
    { repository: "new-owner/project", number: 7, description: "Original description", firstRecordedIn: "seed", lastSeenInSearch: true },
  ], [], aliases);

  assert.equal(result.length, 1);
  assert.equal(result[0].repository, "new-owner/project");
  assert.equal(result[0].repositoryId, "42");
  assert.equal(result[0].number, 7);
  assert.equal(result[0].firstRecordedIn, "seed");
  assert.equal(result[0].lastSeenInSearch, true);
});

test("does not merge equal PR numbers from unrelated repositories", () => {
  const result = mergeContributions([
    { repository: "owner-one/project", repositoryId: "101", number: 12, description: "Same title" },
    { repository: "owner-two/project", repositoryId: "202", number: 12, description: "Same title" },
  ]);

  assert.equal(result.length, 2);
  assert.notEqual(contributionIdentity(result[0]), contributionIdentity(result[1]));
});

test("keeps unavailable historical repositories using their legacy identity", () => {
  const result = mergeContributions([
    { repository: "deleted-owner/project", number: 3, description: "Retained history", firstRecordedIn: "seed" },
  ], [], new Map([["deleted-owner/project", null]]));

  assert.equal(result.length, 1);
  assert.equal(result[0].repository, "deleted-owner/project");
  assert.equal(result[0].description, "Retained history");
  assert.equal(result[0].repositoryId, null);
});

test("discovered PRs enrich existing records without replacing curated descriptions", () => {
  const existing = [
    { repository: "old-owner/project", repositoryId: "42", number: 7, description: "Curated description", firstRecordedIn: "seed", lastSeenInSearch: false },
  ];
  const discovered = [
    { repository: "new-owner/project", repositoryId: "42", number: 7, description: "Raw upstream title", firstRecordedIn: "GitHub Search API", pullRequestId: "9001", pullRequestNodeId: "PR_node", lastSeenInSearch: true },
  ];

  const result = mergeContributions(existing, discovered, aliases);
  assert.equal(result.length, 1);
  assert.equal(result[0].description, "Curated description");
  assert.equal(result[0].firstRecordedIn, "seed");
  assert.equal(result[0].pullRequestId, "9001");
  assert.equal(result[0].pullRequestNodeId, "PR_node");
  assert.equal(result[0].lastSeenInSearch, true);
});

test("repeated merges are idempotent", () => {
  const existing = [
    { repository: "old-owner/project", number: 7, description: "PR 7", firstRecordedIn: "seed" },
  ];
  const discovered = [
    { repository: "new-owner/project", number: 7, description: "PR 7", pullRequestId: "9001" },
    { repository: "new-owner/project", number: 8, description: "PR 8", pullRequestId: "9002" },
  ];

  const first = mergeContributions(existing, discovered, aliases);
  const second = mergeContributions(first, discovered, aliases);
  assert.deepEqual(second, first);
});
