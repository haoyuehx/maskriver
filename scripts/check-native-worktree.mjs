// Main-only preflight for the installed pi-subagents 0.76.1 native allocator.
// No model/agent launch and no merge/push. Not a fallback runner.
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { registerHooks } from 'node:module';
import { pathToFileURL } from 'node:url';
import { execFileSync } from 'node:child_process';

const repo = '/home/haoyue/Project/maskriver';
const base = '/home/haoyue/Project/worktrees';
const extension = '/home/haoyue/.pi/agent/npm/node_modules/pi-subagents';
const host = '/home/haoyue/.pi/agent/install/releases/1.1.0/node_modules/@earendil-works';
assert.equal(fs.realpathSync(process.cwd()), repo, 'run from the main MaskRiver checkout');
assert.equal(JSON.parse(fs.readFileSync(path.join(extension, 'package.json'))).version, '0.76.1', 're-review native APIs after upgrades');
// Pi normally supplies these peer packages. Resolve the existing host copies;
// do not install modules, patch the extension, or substitute a different runner.
const aliases = new Map(['pi-coding-agent', 'pi-tui', 'pi-ai', 'pi-agent-core'].map(name =>
  [`@earendil-works/${name}`, pathToFileURL(path.join(host, name, 'dist/index.js')).href]));
registerHooks({ resolve(specifier, context, next) { return next(aliases.get(specifier) ?? specifier, context); } });
const { loadConfig, getConfigPath } = await import(pathToFileURL(path.join(extension, 'src/extension/config.js')));
const native = await import(pathToFileURL(path.join(extension, 'src/runs/shared/worktree.js')));
const config = loadConfig();
assert.equal(config.worktreeProvider, 'native');
assert.equal(config.worktreeBaseDir, base);
assert.equal(native.resolveWorktreeProvider(config.worktreeProvider, config.worktreeBaseDir), 'native');
const planned = native.resolveExpectedWorktreeAgentCwd(repo, 'config-probe', 0, config.worktreeBaseDir);
assert.ok(planned.startsWith(`${base}/maskriver/`));
assert.throws(() => native.resolveExpectedWorktreeAgentCwd(repo, 'unsafe-probe', 0, `${repo}/.local/worktrees`));
const oldEnv = process.env.PI_SUBAGENTS_WORKTREE_DIR;
try {
  process.env.PI_SUBAGENTS_WORKTREE_DIR = '/tmp/maskriver-precedence-only-no-files';
  assert.equal(native.resolveExpectedWorktreeAgentCwd(repo, 'config-probe', 0, config.worktreeBaseDir), planned);
  assert.ok(native.resolveExpectedWorktreeAgentCwd(repo, 'env-probe', 0).startsWith('/tmp/maskriver-precedence-only-no-files/'));
  assert.equal(native.resolveWorktreeProvider('auto', config.worktreeBaseDir), 'native');
  assert.throws(() => native.resolveWorktreeProvider('worktrunk', config.worktreeBaseDir));
} finally {
  if (oldEnv === undefined) delete process.env.PI_SUBAGENTS_WORKTREE_DIR;
  else process.env.PI_SUBAGENTS_WORKTREE_DIR = oldEnv;
}
console.log(JSON.stringify({ configPath: getConfigPath(), provider: 'native', base, planned, precedenceChecks: 'pass' }, null, 2));
if (process.argv.length === 2) process.exit(0);
assert.deepEqual(process.argv.slice(2), ['--allocate'], 'only --allocate is supported');
const git = (cwd, ...args) => execFileSync('git', ['-C', cwd, ...args], { encoding: 'utf8' }).trim();
assert.equal(git(repo, 'status', '--porcelain'), '', 'commit the reviewed baseline before allocation');
const head = git(repo, 'rev-parse', 'HEAD');
const runID = `main-native-probe-${Date.now()}`;
const evidence = path.join(repo, '.local', runID);
fs.mkdirSync(evidence, { recursive: true, mode: 0o700 });
let setup;
try {
  setup = await native.createWorktrees(repo, runID, 1, {
    provider: config.worktreeProvider, baseDir: config.worktreeBaseDir,
    baseRef: 'refs/heads/main', agents: ['main-preflight'],
  });
  fs.writeFileSync(path.join(evidence, 'setup.json'), JSON.stringify(setup, null, 2));
  const worktree = setup.worktrees[0];
  assert.equal(worktree.provider, 'native');
  assert.ok(fs.realpathSync(worktree.path).startsWith(`${base}/maskriver/`));
  assert.notEqual(worktree.path, repo);
  assert.equal(git(worktree.path, 'rev-parse', 'HEAD'), head);
  assert.equal(fs.realpathSync(git(worktree.path, 'rev-parse', '--git-common-dir')), path.join(repo, '.git'));
  const relative = 'internal/db/native_probe_test.go';
  assert.ok(!fs.existsSync(path.join(worktree.path, relative)));
  assert.ok(!fs.existsSync(path.join(repo, relative)));
  fs.writeFileSync(path.join(worktree.path, relative), 'package db\n\nimport ("testing"; "github.com/haoyuehx/maskriver/pkg/contracts")\n\nfunc TestNativeProbe(t *testing.T) { if contracts.APIRevision != "m1a-v1" { t.Fatal("unexpected contract revision") } }\n');
  execFileSync('gofmt', ['-w', relative], { cwd: worktree.path });
  const testLog = execFileSync('go', ['test', '-count=1', './...'], { cwd: worktree.path, encoding: 'utf8', timeout: 180000 });
  fs.writeFileSync(path.join(evidence, 'tests.log'), testLog);
  const diffs = native.diffWorktrees(setup, ['main-preflight'], path.join(evidence, 'patches'));
  assert.equal(diffs.length, 1);
  assert.equal(diffs[0].error, undefined);
  assert.equal(diffs[0].filesChanged, 1);
  assert.equal(native.validateWorktreePatchRepresentsCurrentWorktree(worktree.path, head, diffs[0].patchPath), undefined);
  fs.writeFileSync(path.join(evidence, 'diffs.json'), JSON.stringify(diffs, null, 2));
  // Remove only the exact synthetic file created above, after its patch is saved.
  // Native capture stages changes; Main unstages this one file before removing it.
  git(worktree.path, 'restore', '--staged', '--', relative);
  fs.unlinkSync(path.join(worktree.path, relative));
  assert.equal(git(worktree.path, 'status', '--porcelain'), '');
  const cleanup = native.cleanupWorktrees(setup, { kind: 'preserve' });
  fs.writeFileSync(path.join(evidence, 'cleanup.json'), JSON.stringify(cleanup, null, 2));
  assert.equal(cleanup.state, 'complete');
  assert.ok(!fs.existsSync(worktree.path));
  assert.equal(git(repo, 'rev-parse', 'HEAD'), head);
  assert.equal(git(repo, 'status', '--porcelain'), '');
  console.log(JSON.stringify({ result: 'PASS', evidence, baseCommit: head, modelCalled: false, agentLaunched: false, merged: false }, null, 2));
} catch (error) {
  // Keep uncertain state rather than force-cleaning an unverified patch or tree.
  fs.writeFileSync(path.join(evidence, 'failure.txt'), String(error));
  console.error(`FAIL: inspect ${evidence}; no fallback or automatic merge`);
  throw error;
}
