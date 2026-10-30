import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { test } from "node:test";

const workflow = readFileSync(
  new URL("../workflows/dependabot-automerge.yml", import.meta.url),
  "utf8"
);
const [, source] = workflow.split("          script: |\n");
if (!source) {
  throw new Error("The tests must execute the script shipped in the workflow.");
}
const script = source
  .split("\n")
  .map((line) => line.slice(12))
  .join("\n");
const AsyncFunction = Object.getPrototypeOf(async () => {
  await Promise.resolve();
}).constructor;
const execute = new AsyncFunction(
  "github",
  "context",
  "core",
  "process",
  script
);
const HEAD_MISMATCH = /Head SHA mismatch/;
const APPROVAL_DENIED = /Approval denied/;
const MERGE_BLOCKED = /Merge requirements are not satisfied/;
const INVALID_PR = /pull request number/;
const INVALID_SHA = /full commit SHA/;
const MISSING_CREDENTIAL = /DEPENDABOT_AUTOMERGE_TOKEN/;

const sha = "a".repeat(40);
const otherSha = "b".repeat(40);

function fixture() {
  return {
    pr: {
      number: 7,
      state: "open",
      draft: false,
      user: { login: "dependabot[bot]" },
      head: { sha, repo: { full_name: "DefangLabs/example" } },
    },
    runs: [
      {
        id: 100,
        workflow_id: 1,
        event: "pull_request",
        check_suite_id: 10,
        path: ".github/workflows/ci.yml",
        status: "completed",
        conclusion: "success",
      },
    ],
    checks: [
      {
        name: "test",
        check_suite: { id: 10 },
        status: "completed",
        conclusion: "success",
      },
    ],
    statuses: [],
    reviews: [],
  };
}

async function run(data = fixture(), options = {}) {
  const writes = [];
  let reads = 0;
  let merged = false;
  const get = (args) => {
    if (data.prs) {
      return {
        data: structuredClone(
          data.prs.find((candidate) => candidate.number === args.pull_number)
        ),
      };
    }
    const snapshots = data.snapshots || [data.pr];
    const pr = snapshots[Math.min(reads, snapshots.length - 1)];
    reads += 1;
    return { data: structuredClone(pr) };
  };
  const github = {
    rest: {
      repos: {
        listPullRequestsAssociatedWithCommit: "candidates",
        listCommitStatusesForRef: "statuses",
      },
      actions: { listWorkflowRunsForRepo: "runs" },
      checks: { listForRef: "checks" },
      pulls: {
        get,
        list: "candidates",
        listReviews: "reviews",
        createReview: (args) => {
          if (data.approvalError) {
            throw new Error("Approval denied");
          }
          writes.push({ type: "approve", ...args });
        },
        merge: (args) => {
          writes.push({ type: "merge", ...args });
          if (data.liveSha && data.liveSha !== args.sha) {
            throw new Error("Head SHA mismatch");
          }
          merged = !(
            data.mergeBlocked || data.blockedPRs?.includes(args.pull_number)
          );
          return {
            data: { merged, message: "Merge requirements are not satisfied" },
          };
        },
      },
    },
    paginate: (route, args) => {
      if (args.per_page !== 100) {
        throw new Error("Pagination must request full pages.");
      }
      if (route === "candidates") {
        return data.noCandidates ? [] : data.prs || [data.pr];
      }
      if (!["runs", "checks", "statuses", "reviews"].includes(route)) {
        throw new Error("Unexpected API route.");
      }
      return structuredClone(data[route]);
    },
  };
  const context = {
    repo: { owner: "DefangLabs", repo: "example" },
    eventName:
      options.event ||
      (options.dispatch ? "workflow_dispatch" : "workflow_run"),
    payload: options.dispatch
      ? {
          inputs: {
            "pull-request": options.number || "7",
            "dry-run": options.dryRun || "false",
          },
        }
      : { workflow_run: { head_sha: options.sha || sha } },
  };
  if (options.event === "check_run") {
    context.payload = { check_run: { head_sha: sha } };
  }
  if (options.event === "status") {
    context.payload = { sha };
  }
  if (options.event === "schedule") {
    context.payload = {};
  }
  const messages = [];
  let error;
  try {
    await execute(
      github,
      context,
      {
        info: (message) => messages.push(message),
        error: (message) => messages.push(message),
      },
      {
        env: {
          MERGE_CREDENTIAL_AVAILABLE:
            options.credential === false ? "false" : "true",
        },
      }
    );
  } catch (caught) {
    if (!options.captureError) {
      throw caught;
    }
    error = caught;
  }
  return { writes, merged, messages, error };
}

async function remainsOpen(data, options) {
  const result = await run(data, options);
  if (result.writes.length || result.merged) {
    throw new Error(
      `Unexpected merge or approval: ${JSON.stringify(result.writes)}`
    );
  }
}

test("a green Dependabot PR is approved and squash merged at its exact SHA, without a version restriction", async () => {
  const result = await run();
  assert.equal(result.merged, true);
  assert.deepEqual(
    result.writes.map((write) => write.type),
    ["approve", "merge"]
  );
  assert.equal(result.writes[0].commit_id, sha);
  assert.equal(result.writes[1].sha, sha);
  assert.equal(result.writes[1].merge_method, "squash");
});

for (const [name, mutate] of [
  [
    "human author",
    (data) => {
      data.pr.user.login = "maintainer";
    },
  ],
  [
    "draft",
    (data) => {
      data.pr.draft = true;
    },
  ],
  [
    "closed",
    (data) => {
      data.pr.state = "closed";
    },
  ],
  [
    "fork",
    (data) => {
      data.pr.head.repo.full_name = "elsewhere/example";
    },
  ],
  [
    "missing head repository",
    (data) => {
      data.pr.head.repo = null;
    },
  ],
  [
    "outdated CI commit",
    (data) => {
      data.pr.head.sha = otherSha;
    },
  ],
]) {
  test(`${name} never approves or merges`, async () => {
    const data = fixture();
    mutate(data);
    await remainsOpen(data);
  });
}

for (const conclusion of [
  "failure",
  "cancelled",
  "timed_out",
  "action_required",
  "stale",
  null,
]) {
  test(`${conclusion} check leaves the PR open`, async () => {
    const data = fixture();
    data.checks[0].conclusion = conclusion;
    await remainsOpen(data);
  });
}

for (const status of ["queued", "in_progress", "waiting", "pending"]) {
  test(`${status} check leaves the PR open`, async () => {
    const data = fixture();
    data.checks[0].status = status;
    await remainsOpen(data);
  });
}

for (const state of ["pending", "failure", "error"]) {
  test(`${state} legacy commit status blocks otherwise-green checks`, async () => {
    const data = fixture();
    data.statuses = [{ id: 1, context: "external-ci", state }];
    await remainsOpen(data);
  });
}

test("a queued workflow blocks merging even before its checks are created", async () => {
  const data = fixture();
  data.runs.push({
    id: 200,
    workflow_id: 2,
    event: "pull_request",
    status: "queued",
    conclusion: null,
  });
  await remainsOpen(data);
});

test("a failed workflow blocks merging even if its visible job is green", async () => {
  const data = fixture();
  data.runs[0].conclusion = "failure";
  await remainsOpen(data);
});

test("no check results is not green", async () => {
  const data = fixture();
  data.checks = [];
  await remainsOpen(data);
});

test("an entirely skipped or neutral check suite is not green", async () => {
  const data = fixture();
  data.checks[0].conclusion = "neutral";
  await remainsOpen(data);
});

test("neutral and skipped checks can accompany successful CI", async () => {
  const data = fixture();
  data.checks.push(
    { status: "completed", conclusion: "neutral" },
    { status: "completed", conclusion: "skipped" }
  );
  assert.equal((await run(data)).merged, true);
});

test("a green legacy status can supply the success signal", async () => {
  const data = fixture();
  data.checks = [];
  data.statuses = [{ id: 1, context: "external-ci", state: "success" }];
  assert.equal((await run(data)).merged, true);
});

for (const state of ["success", "failure"]) {
  test(`the newest legacy status (${state}) wins independently of array order`, async () => {
    const data = fixture();
    data.statuses = [
      { id: 1, context: "external-ci", state: "failure" },
      { id: 2, context: "external-ci", state },
    ];
    assert.equal((await run(data)).merged, state === "success");
  });
}

test("a successful workflow rerun replaces its older failed suite", async () => {
  const data = fixture();
  data.runs.push({
    ...data.runs[0],
    id: 90,
    check_suite_id: 9,
    conclusion: "failure",
  });
  data.checks.push({
    check_suite: { id: 9 },
    status: "completed",
    conclusion: "failure",
  });
  assert.equal((await run(data)).merged, true);
});

test("a pending replacement workflow prevents using its previous green suite", async () => {
  const data = fixture();
  data.runs.push({
    ...data.runs[0],
    id: 101,
    check_suite_id: 11,
    status: "queued",
    conclusion: null,
  });
  await remainsOpen(data);
});

test("different CI events for one workflow are each enforced", async () => {
  const data = fixture();
  data.runs.push({
    ...data.runs[0],
    id: 101,
    event: "push",
    conclusion: "failure",
  });
  await remainsOpen(data);
});

test("obsolete auto-merge checks cannot deadlock the replacement policy", async () => {
  const data = fixture();
  data.runs.push({
    id: 200,
    workflow_id: 2,
    event: "pull_request",
    check_suite_id: 20,
    path: ".github/workflows/dependabot-automerge.yml",
    status: "in_progress",
    conclusion: null,
  });
  data.checks.push({
    check_suite: { id: 20 },
    status: "in_progress",
    conclusion: null,
  });
  assert.equal((await run(data)).merged, true);
});

test("third-party failures are enforced independently of Actions workflows", async () => {
  const data = fixture();
  data.checks.push({ status: "completed", conclusion: "failure" });
  await remainsOpen(data);
});

test("a failing check after the first 100 results still blocks merging", async () => {
  const data = fixture();
  data.checks = Array.from({ length: 100 }, () => ({
    status: "completed",
    conclusion: "success",
  }));
  data.checks.push({ status: "completed", conclusion: "failure" });
  await remainsOpen(data);
});

test("a new head discovered immediately before approval is left alone", async () => {
  const data = fixture();
  const fresh = structuredClone(data.pr);
  fresh.head.sha = otherSha;
  data.snapshots = [data.pr, fresh];
  await remainsOpen(data);
});

test("a newly drafted PR is left alone", async () => {
  const data = fixture();
  const fresh = structuredClone(data.pr);
  fresh.draft = true;
  data.snapshots = [data.pr, fresh];
  await remainsOpen(data);
});

test("the merge SHA guard rejects a push occurring after the last PR read", async () => {
  const data = fixture();
  data.liveSha = otherSha;
  await assert.rejects(run(data), HEAD_MISMATCH);
});

test("an existing bot approval at this SHA is reused", async () => {
  const data = fixture();
  data.reviews = [
    {
      user: { login: "maintenance-app[bot]" },
      commit_id: sha,
      state: "APPROVED",
      body: "Approved automatically after CI passed for this Dependabot update.",
    },
  ];
  const result = await run(data);
  assert.deepEqual(
    result.writes.map((write) => write.type),
    ["merge"]
  );
});

test("an approval from an older SHA does not approve the current change", async () => {
  const data = fixture();
  data.reviews = [
    {
      user: { login: "maintenance-app[bot]" },
      commit_id: otherSha,
      state: "APPROVED",
      body: "Approved automatically after CI passed for this Dependabot update.",
    },
  ];
  assert.equal((await run(data)).writes[0].type, "approve");
});

test("an approval permission failure never reaches merge", async () => {
  const data = fixture();
  data.approvalError = true;
  await assert.rejects(run(data), APPROVAL_DENIED);
});

test("GitHub merge protections remain authoritative", async () => {
  const data = fixture();
  data.mergeBlocked = true;
  await assert.rejects(run(data), MERGE_BLOCKED);
});

test("manual dispatch uses the current Dependabot head", async () => {
  assert.equal((await run(fixture(), { dispatch: true })).merged, true);
});

test("manual dispatch cannot authorize a human PR", async () => {
  const data = fixture();
  data.pr.user.login = "maintainer";
  await remainsOpen(data, { dispatch: true });
});

test("malformed manual input is rejected", async () => {
  await assert.rejects(
    run(fixture(), { dispatch: true, number: "7; echo unsafe" }),
    INVALID_PR
  );
});

test("an invalid commit SHA is rejected", async () => {
  await assert.rejects(run(fixture(), { sha: "invalid" }), INVALID_SHA);
});

test("unrelated workflow completion cannot merge a PR", async () => {
  const data = fixture();
  data.noCandidates = true;
  await remainsOpen(data);
});

for (const event of ["check_run", "status", "schedule"]) {
  test(`${event} rechecks a PR after external checks complete`, async () => {
    assert.equal((await run(fixture(), { event })).merged, true);
  });
}

test("an hourly retry uses the current head, not an old event SHA", async () => {
  const data = fixture();
  data.pr.head.sha = otherSha;
  const result = await run(data, { event: "schedule" });
  assert.equal(result.writes[1].sha, otherSha);
});

test("a dry run never writes even when every check is green", async () => {
  const result = await run(fixture(), {
    dispatch: true,
    dryRun: "true",
    credential: false,
  });
  assert.deepEqual(result.writes, []);
  assert.equal(result.merged, false);
  assert.ok(
    result.messages.some((message) => message.startsWith("Would merge"))
  );
});

test("a missing maintenance credential fails closed instead of merging with GITHUB_TOKEN", async () => {
  await assert.rejects(
    run(fixture(), { credential: false }),
    MISSING_CREDENTIAL
  );
});

test("a blocked PR does not starve the rest of the hourly sweep", async () => {
  const data = fixture();
  const second = structuredClone(data.pr);
  second.number = 8;
  second.head.sha = otherSha;
  data.prs = [data.pr, second];
  data.blockedPRs = [7];
  const result = await run(data, { event: "schedule", captureError: true });
  assert.ok(result.error);
  assert.ok(
    result.writes.some(
      (write) => write.type === "merge" && write.pull_number === 8
    )
  );
  assert.ok(result.messages.some((message) => message.startsWith("PR #7:")));
});

test("a dismissed maintenance approval is not reused", async () => {
  const data = fixture();
  data.reviews = [
    {
      id: 1,
      user: { login: "maintenance-app[bot]" },
      commit_id: sha,
      state: "DISMISSED",
      body: "Approved automatically after CI passed for this Dependabot update.",
    },
  ];
  assert.equal((await run(data)).writes[0].type, "approve");
});

test("the latest verdict replaces an older approval by the same maintenance identity", async () => {
  const data = fixture();
  data.reviews = [
    {
      id: 1,
      user: { login: "maintenance-app[bot]" },
      commit_id: sha,
      state: "APPROVED",
      body: "Approved automatically after CI passed for this Dependabot update.",
    },
    {
      id: 2,
      user: { login: "maintenance-app[bot]" },
      commit_id: sha,
      state: "CHANGES_REQUESTED",
      body: "Recheck",
    },
  ];
  assert.equal((await run(data)).writes[0].type, "approve");
});
