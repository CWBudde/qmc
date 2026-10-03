"""Shared release version, metadata, source identity, and review-attestation rules."""

import os
from pathlib import Path
import re
import subprocess

ROOT = Path(__file__).resolve().parent.parent
SHA = re.compile(r"[0-9a-fA-F]{40}\Z")
VERSION = re.compile(r"(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)"
                     r"(?:-([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?"
                     r"(?:\+([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?\Z")


def normalize_version(value):
    value = value.removeprefix("version=").removeprefix("v")
    match = VERSION.fullmatch(value)
    if not match or (match[4] and any(part.isdigit() and len(part) > 1 and part[0] == "0"
                                     for part in match[4].split("."))):
        raise ValueError("Invalid semantic version; use 1.2.3, 1.2.3-rc.1, or 1.2.3+build.1")
    return value


def git(*arguments):
    return subprocess.check_output(["git", "-C", str(ROOT), *arguments], text=True, timeout=60).strip()


def source(reviewed_commit):
    if not SHA.fullmatch(reviewed_commit):
        raise ValueError("Reviewed commit must be its full 40-character SHA")
    reviewed_commit = reviewed_commit.lower()
    head = git("rev-parse", "HEAD")
    if head != reviewed_commit:
        raise ValueError("HEAD differs from the explicitly reviewed commit")
    if git("status", "--porcelain", "--untracked-files=all"):
        raise ValueError("Release validation requires a clean worktree, including untracked files")
    expected = os.environ.get("QMC_RELEASE_EXPECTED_SHA")
    if expected:
        if not SHA.fullmatch(expected) or git("rev-parse", expected + "^{commit}") != head:
            raise ValueError("Checkout differs from the workflow event commit")
    return head


def metadata(version):
    for name in ("LICENSE", "README.md", "CHANGELOG.md"):
        if not (ROOT / name).read_text().strip():
            raise ValueError(f"Release metadata is empty: {name}")
    changelog = (ROOT / "CHANGELOG.md").read_text()
    sections = re.findall(r"^## \[([^\]]+)\](?:[^\n]*)\n(.*?)(?=^## |\Z)", changelog, re.M | re.S)
    entries = [body for title, body in sections if title == version]
    if len(entries) != 1 or not re.search(r"^[-*] \S", entries[0], re.M):
        raise ValueError("Release needs one exact changelog version section with documented changes")
    root_module = (ROOT / "go.mod").read_text()
    demo_module = (ROOT / "examples/wasm-demo/go.mod").read_text()
    if not re.search(r"^module github\.com/cwbudde/qmc\s*$", root_module, re.M):
        raise ValueError("Unexpected root module path")
    if not re.search(r"^module wasm-demo\s*$", demo_module, re.M):
        raise ValueError("Unexpected demo module path")
    if not re.search(r"^replace github\.com/cwbudde/qmc => ../../\s*$", demo_module, re.M):
        raise ValueError("Demo module must use the reviewed local library")
    if version.split(".", 1)[0] not in ("0", "1"):
        raise ValueError("Release majors >=2 require a versioned Go module path")
    tag = "refs/tags/v" + version
    exists = subprocess.run(["git", "-C", str(ROOT), "show-ref", "--verify", "--quiet", tag], timeout=60)
    if exists.returncode == 0 and git("rev-parse", tag + "^{commit}") != git("rev-parse", "HEAD"):
        raise ValueError("Version already identifies a different source commit")
    if exists.returncode not in (0, 1):
        raise subprocess.CalledProcessError(exists.returncode, exists.args)


def tag_attestation(version):
    tag = "refs/tags/v" + version
    if git("cat-file", "-t", tag) != "tag":
        raise ValueError("Release tag must be annotated with a Reviewed-Commit trailer")
    reviewed = git("for-each-ref", "--format=%(contents:trailers:key=Reviewed-Commit,valueonly)", tag).splitlines()
    if len(reviewed) != 1 or not SHA.fullmatch(reviewed[0]):
        raise ValueError("Release tag needs exactly one Reviewed-Commit trailer")
    head = source(reviewed[0])
    if git("rev-parse", tag + "^{commit}") != head:
        raise ValueError("Release tag differs from the reviewed checkout")
    subprocess.run(["git", "-C", str(ROOT), "merge-base", "--is-ancestor", head, "origin/main"],
                   check=True, timeout=60)
    return head


def workflow_version():
    event = os.environ.get("QMC_RELEASE_EVENT")
    if event == "workflow_dispatch":
        version = normalize_version(os.environ.get("QMC_RELEASE_INPUT_VERSION", ""))
        reviewed = source(os.environ.get("QMC_RELEASE_REVIEWED_COMMIT", ""))
    elif event == "push":
        ref = os.environ.get("QMC_RELEASE_REF", "")
        if not ref.startswith("refs/tags/v"):
            raise ValueError("Release push event must refer to a version tag")
        version = normalize_version(ref.removeprefix("refs/tags/"))
        reviewed = tag_attestation(version)
    else:
        raise ValueError("Unsupported release event")
    metadata(version)
    return version, reviewed
