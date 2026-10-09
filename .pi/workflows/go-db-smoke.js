// Prepared only. Main must explicitly approve this bounded smoke, not a migration.
if (args.confirm !== "run-go-db-smoke") throw new Error("explicit smoke approval required");
return await runs.run("go-db-smoke", {
  label: "Check isolated Go database workflow",
  agent: "go-db",
  model: "openai-codex/gpt-5.6-sol",
  context: "fresh",
  worktree: true,
  baseRef: "refs/heads/main",
  gate: "go test -count=1 ./... && go vet ./...",
  output: "smoke/go-db.md",
  outputMode: "file-only",
  task: "This is a tiny authorized worktree/model smoke, NOT the go-db component task. Read AGENTS.md and docs/contracts.md. Before writing, run pwd and read-only git checks. Your cwd must be a distinct native worktree under /home/haoyue/Project/worktrees/maskriver/pi-worktree-*, not the main checkout. Its git common directory must be /home/haoyue/Project/maskriver/.git and HEAD must match the Main-provided frozen baseline; stop on mismatch or absent worktree metadata. Only create internal/db/worktree_smoke_test.go (package db_test). Write a small test asserting contracts.APIRevision is m1a-v1 and opening a new SQLite file in t.TempDir via the already pinned modernc driver, using invented constants to test a tiny create/insert/select/rollback sequence. No business adapter implementation, MySQL connection, new dependency or DTO. Do not modify any other project source, docs, shared packages, go.mod/go.sum or .pi. Run gofmt on the new file, go test -count=1 ./..., and go vet ./.... Do not git add/commit/push/merge, create branches, invoke another agent or install anything. Return exact cwd, git-common-dir, branch, base HEAD, changed path, test commands/results and risks through the bound report output. Model identity must be verified by runtime evidence, not your own assertion. Main retains and validates the patch; this diagnostic test is NOT to be automatically merged into main. Stop if isolation, model, tools or tests fail; no execution-mode fallback."
});
