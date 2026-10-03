#!/usr/bin/env python3
"""Exercise release policy and orchestration in private, offline Git fixtures."""

import importlib.util
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import time

from release_common import normalize_version
from check_workflows import check, load

ROOT = Path(__file__).resolve().parent.parent
JUST = shutil.which("just")
assert JUST, "just is required"

workflows = load()
check(workflows)
for name, before, after in [
    ("test.yml", "actions/checkout@11d5960a326750d5838078e36cf38b85af677262", "actions/checkout@v4"),
    ("test.yml", "contents: read", "contents: write"),
    ("test.yml", "persist-credentials: false", "persist-credentials: true"),
    ("test.yml", "timeout-minutes: 15", "timeout-minutes: 0"),
    ("wasm-demo-pages.yml", "needs: build", "needs: missing"),
    ("wasm-demo-pages.yml", "      pages: write", "      contents: write"),
    ("wasm-demo-pages.yml", "    name: Build demo", "    permissions:\n      pages: write\n    name: Build demo"),
]:
    changed = dict(workflows)
    assert before in changed[name], (name, before)
    changed[name] = changed[name].replace(before, after, 1)
    try:
        check(changed)
    except ValueError:
        pass
    else:
        raise AssertionError(f"workflow policy accepted {name}: {after}")

for version in ["0.0.0", "0.4.0", "1.2.3-rc.1", "1.2.3-0", "1.2.3-01a", "1.2.3+01.build", "v1.2.3", "version=v1.2.3"]:
    assert normalize_version(version) == version.removeprefix("version=").removeprefix("v")
for version in ["01.2.3", "1.02.3", "1.2.03", "1.2", "1.2.3-", "1.2.3-01", "1.2.3-a..b", "1.2.3+", "1.2.3+a..b", "1.2.3\n", "vv1.2.3", "1.2.3/evil", "1.2.3_foo", "١.2.3"]:
    try:
        normalize_version(version)
    except ValueError:
        pass
    else:
        raise AssertionError(f"invalid semantic version accepted: {version!r}")

with tempfile.TemporaryDirectory(prefix="qmc-release-gates-") as directory:
    temporary = Path(directory)
    copy = temporary / "source"
    scripts = copy / "scripts"
    scripts.mkdir(parents=True)
    for name in ["release.sh", "verify-release.sh", "release_common.py", "release-policy.py", "run_release_gates.py"]:
        shutil.copy(ROOT / "scripts" / name, scripts / name)
    shutil.copy(ROOT / "justfile", copy / "justfile")
    shutil.copytree(ROOT / "tools", copy / "tools")
    shutil.copy(ROOT / "LICENSE", copy / "LICENSE")
    shutil.copy(ROOT / "go.mod", copy / "go.mod")
    demo = copy / "examples/wasm-demo"
    demo.mkdir(parents=True)
    shutil.copy(ROOT / "examples/wasm-demo/go.mod", demo / "go.mod")
    (copy / ".gitignore").write_text("__pycache__/\n")
    (copy / "README.md").write_text("Reviewed release fixture\n")
    changes = "# Changelog\n\n## [Unreleased]\n\n- Next work.\n\n## [0.4.0] - 2026-10-03\n\n- Documented change.\n"
    (copy / "CHANGELOG.md").write_text(changes)
    mock = temporary / "tools/bin"
    mock.mkdir(parents=True)
    log = temporary / "gates.jsonl"
    mock_just = mock / "just"
    mock_just.write_text(f"#!{sys.executable}\n" + '''
import json, os, pathlib, signal, subprocess, sys, time
recipe=sys.argv[1]
with open(os.environ['QMC_TEST_GATE_LOG'],'a') as log:
    log.write(json.dumps({'recipe':recipe,'go':os.environ.get('GOTOOLCHAIN'),'gowork':os.environ.get('GOWORK'),'flags':os.environ.get('GOFLAGS')})+'\\n')
if recipe in ['slow','failed-child']:
    child=subprocess.Popen([sys.executable,'-c','import pathlib,signal,time,sys;signal.signal(signal.SIGTERM,signal.SIG_IGN);time.sleep(0.7);pathlib.Path(sys.argv[1]).write_text("orphan")',os.environ['QMC_TEST_ORPHAN']])
    pathlib.Path(os.environ['QMC_TEST_CHILD']).write_text(str(child.pid))
    if recipe=='failed-child':sys.exit(23)
    time.sleep(5)
if os.environ.get('QMC_TEST_GATE_FAIL')==recipe:sys.exit(23)
if recipe=='ci' and os.environ.get('QMC_TEST_DRIFT'):
    pathlib.Path('README.md').write_text('changed during checks')
if recipe=='ci' and os.environ.get('QMC_TEST_COMMIT_DRIFT'):
    subprocess.run(['git','commit','--allow-empty','-m','unexpected source change'],check=True,stdout=subprocess.DEVNULL)
if recipe=='ci' and os.environ.get('QMC_TEST_REMOTE_ADVANCE'):
    subprocess.run(['git','--git-dir',os.environ['QMC_TEST_REMOTE_GIT'],'update-ref','refs/heads/main',os.environ['QMC_TEST_REMOTE_ADVANCE']],check=True)
''')
    mock_just.chmod(0o755)
    environment = {**os.environ, "GIT_CONFIG_GLOBAL": os.devnull, "GIT_CONFIG_NOSYSTEM": "1",
                   "QMC_TOOLS_DIR": str(mock.parent), "QMC_TEST_GATE_LOG": str(log),
                   "PATH": str(mock) + os.pathsep + os.environ["PATH"],
                   "GOTOOLCHAIN": "go1.23.0", "GOWORK": "/external/workspace", "GOFLAGS": "-tags=unexpected"}

    def git(*arguments):
        return subprocess.check_output(["git", "-C", str(copy), *arguments], env=environment, text=True).strip()

    git("init", "-q", "-b", "main")
    git("config", "user.name", "QMC release tests")
    git("config", "user.email", "fixture@example.invalid")
    git("config", "commit.gpgsign", "false")
    git("config", "tag.gpgsign", "false")
    git("config", "core.hooksPath", os.devnull)
    git("add", ".")
    git("commit", "-qm", "Reviewed fixture")
    head = git("rev-parse", "HEAD")
    remote = temporary / "remote.git"
    subprocess.run(["git", "init", "--bare", "-q", str(remote)], env=environment, check=True)
    git("remote", "add", "origin", str(remote))
    git("push", "-q", "origin", "main")

    def run(recipe="release-check", version="0.4.0", reviewed=head, **extra):
        return subprocess.run([JUST, recipe, version, reviewed], cwd=copy,
                              env={**environment, **extra}, capture_output=True, text=True, timeout=30)

    def success(result):
        assert result.returncode == 0, result.stdout + result.stderr

    success(run(version="version=v0.4.0", reviewed=head.upper()))
    calls = [json.loads(line) for line in log.read_text().splitlines()]
    expected_go = "go" + (ROOT / "tools/go-version").read_text().strip()
    assert [call["recipe"] for call in calls] == ["ci", "test-statistical"], calls
    assert all(call["go"] == expected_go and call["gowork"] == "off" and call["flags"] == "" for call in calls)
    assert not git("tag"), "validation created a tag"
    log.unlink()
    marker = temporary / "injection-marker"
    for payload in ['"; touch ' + str(marker) + '; #', '$(touch ' + str(marker) + ')', '`touch ' + str(marker) + '`']:
        assert run(version=payload).returncode != 0
        assert not marker.exists(), "release argument executed shell code"
    assert not log.exists(), "invalid version ran release gates"
    assert run(reviewed="0" * 40).returncode != 0
    assert run(reviewed=head[:12]).returncode != 0
    assert run(QMC_RELEASE_EXPECTED_SHA="0" * 40).returncode != 0
    (copy / "untracked.txt").write_text("drift")
    assert run().returncode != 0
    (copy / "untracked.txt").unlink()
    saved = (copy / "README.md").read_bytes()
    (copy / "README.md").write_text("uncommitted")
    assert run().returncode != 0
    (copy / "README.md").write_bytes(saved)
    assert not log.exists(), "dirty or different source ran release gates"

    # Changelog matches must be exact, unique and contain actual change entries.
    for bad in [changes.replace("[0.4.0]", "[0.4.01]"), changes.replace("- Documented change.", ""), changes + "\n## [0.4.0]\n\n- Duplicate.\n"]:
        (copy / "CHANGELOG.md").write_text(bad)
        git("add", "CHANGELOG.md"); git("commit", "-qm", "Bad metadata fixture")
        bad_commit = git("rev-parse", "HEAD")
        assert run(reviewed=bad_commit).returncode != 0
        git("reset", "--hard", head)
    for path, bad in [(copy / "LICENSE", ""), (copy / "README.md", ""),
                      (copy / "go.mod", "module unexpected\n"),
                      (demo / "go.mod", (demo / "go.mod").read_text().replace("../../", "/unreviewed/source"))]:
        path.write_text(bad)
        git("add", str(path)); git("commit", "-qm", "Bad package metadata fixture")
        assert run(reviewed=git("rev-parse", "HEAD")).returncode != 0
        git("reset", "--hard", head)
    for recipe in ["ci", "test-statistical"]:
        log.unlink(missing_ok=True)
        assert run(QMC_TEST_GATE_FAIL=recipe).returncode != 0
        assert [json.loads(line)["recipe"] for line in log.read_text().splitlines()] == (["ci"] if recipe == "ci" else ["ci", "test-statistical"])
    git("tag", "v0.4.0", bad_commit)
    log.unlink(missing_ok=True)
    assert run().returncode != 0 and not log.exists(), "version accepted different already-tagged source"
    git("tag", "-d", "v0.4.0")
    assert run(QMC_TEST_DRIFT="1").returncode != 0
    git("reset", "--hard", head)
    assert run(QMC_TEST_COMMIT_DRIFT="1").returncode != 0
    git("reset", "--hard", head)

    # Tags are made only in this private fixture, bound to the reviewed SHA.
    git("checkout", "-qb", "candidate")
    assert run("release").returncode != 0 and not git("tag")
    git("checkout", "-q", "main")
    # A fetched remote head differing from the reviewed local source is refused.
    git("push", "-q", "origin", bad_commit + ":refs/heads/side")
    subprocess.run(["git", "--git-dir", str(remote), "update-ref", "refs/heads/main", bad_commit],
                   env=environment, check=True)
    assert run("release").returncode != 0 and not git("tag")
    subprocess.run(["git", "--git-dir", str(remote), "update-ref", "refs/heads/main", head],
                   env=environment, check=True)
    assert run("release", QMC_TEST_REMOTE_GIT=str(remote), QMC_TEST_REMOTE_ADVANCE=bad_commit).returncode != 0
    assert not git("tag"), "remote advance during verification still created a tag"
    subprocess.run(["git", "--git-dir", str(remote), "update-ref", "refs/heads/main", head],
                   env=environment, check=True)
    success(run("release"))
    assert git("cat-file", "-t", "refs/tags/v0.4.0") == "tag"
    assert git("rev-parse", "v0.4.0^{commit}") == head
    assert git("for-each-ref", "--format=%(contents:trailers:key=Reviewed-Commit,valueonly)", "refs/tags/v0.4.0") == head
    assert run("release").returncode != 0, "existing tag was replaced"

    def workflow(**extra):
        output = temporary / "workflow-output"
        output.unlink(missing_ok=True)
        result = subprocess.run([sys.executable, str(scripts / "release-policy.py"), "--workflow"],
                                env={**environment, "GITHUB_OUTPUT": str(output), "QMC_RELEASE_EXPECTED_SHA": head,
                                     "QMC_RELEASE_EVENT": "push", "QMC_RELEASE_REF": "refs/tags/v0.4.0", **extra},
                                capture_output=True, text=True, timeout=10)
        return result, output.read_text() if output.exists() else ""

    result, outputs = workflow()
    success(result)
    assert outputs == f"version=0.4.0\nreviewed_commit={head}\n"
    success(workflow(QMC_RELEASE_EXPECTED_SHA=git("rev-parse", "refs/tags/v0.4.0"))[0])
    success(workflow(QMC_RELEASE_EVENT="workflow_dispatch", QMC_RELEASE_INPUT_VERSION="v0.4.0", QMC_RELEASE_REVIEWED_COMMIT=head)[0])
    assert workflow(QMC_RELEASE_EXPECTED_SHA="0" * 40)[0].returncode != 0
    assert workflow(QMC_RELEASE_REF="refs/heads/main")[0].returncode != 0
    git("tag", "-d", "v0.4.0")
    git("tag", "v0.4.0")
    assert workflow()[0].returncode != 0, "lightweight tag passed review policy"
    git("tag", "-d", "v0.4.0")
    for message in ["Release", "Reviewed-Commit: " + "0" * 40, f"Reviewed-Commit: {head}\nReviewed-Commit: {head}"]:
        git("tag", "-a", "v0.4.0", "-m", message)
        assert workflow()[0].returncode != 0, message
        git("tag", "-d", "v0.4.0")
    git("tag", "-a", "v0.4.0", "-m", f"Reviewed-Commit: {head}")
    git("update-ref", "-d", "refs/remotes/origin/main")
    assert workflow()[0].returncode != 0, "tag without main ancestry passed"

    # A total deadline must kill descendants that ignore ordinary termination.
    spec = importlib.util.spec_from_file_location("gates", ROOT / "scripts/run_release_gates.py")
    gates = importlib.util.module_from_spec(spec); spec.loader.exec_module(gates)
    gates.GATES, gates.BUDGET_SECONDS = ("slow",), 0.3
    orphan, child = temporary / "orphan-marker", temporary / "child-pid"
    original_environment = dict(os.environ)
    os.environ.update({**environment, "QMC_TEST_ORPHAN": str(orphan), "QMC_TEST_CHILD": str(child)})
    try:
        try:
            gates.run_gates()
        except subprocess.TimeoutExpired:
            pass
        else:
            raise AssertionError("release deadline did not fail")
        assert child.exists(), "timeout fixture never launched its descendant"
        time.sleep(0.8)
        assert not orphan.exists(), "release timeout left a live descendant"
        gates.GATES, gates.BUDGET_SECONDS = ("failed-child",), 10
        try:
            gates.run_gates()
        except subprocess.CalledProcessError as error:
            assert error.returncode == 23
        else:
            raise AssertionError("release child failure did not fail")
        time.sleep(0.8)
        assert not orphan.exists(), "failed release gate left a live descendant"
    finally:
        os.environ.clear(); os.environ.update(original_environment)

print("Release gates passed: strict SemVer, literal arguments, exact documented versions, clean/reviewed source, post-check identity, shared pinned gates, tag attestations, workflow identity, and total deadline cleanup.")
