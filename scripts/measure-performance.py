#!/usr/bin/env python3
"""Record a controlled, repeated local baseline; does not assert universal speed."""

import hashlib
import json
import os
from datetime import datetime, timezone
from pathlib import Path
import platform
import subprocess
import sys

ROOT = Path(__file__).resolve().parent.parent

def capture(command, env):
    result = subprocess.run(command, cwd=ROOT, env=env, text=True,
                            stdout=subprocess.PIPE, stderr=subprocess.STDOUT,
                            timeout=600, check=True)
    return result.stdout

def main():
    if len(sys.argv) != 2:
        sys.exit("usage: measure-performance.py NEW_OUTPUT_DIRECTORY")
    output = Path(sys.argv[1]).resolve()
    output.mkdir(exist_ok=False, parents=True)
    version = (ROOT / "tools/go-version").read_text().strip()
    env = dict(os.environ, GOTOOLCHAIN="go" + version, GOWORK="off", GOFLAGS="", GOMAXPROCS="1")
    command = ["go", "test", "-run", "^$", "-bench",
               "^BenchmarkReview(Point|Construction|Arithmetic|Cache|CacheConstruction|Bulk|Integration)$",
               "-benchmem", "-benchtime=200ms", "-count=5", "-cpu=1", "-timeout=10m", "."]
    metadata = {
        "date": datetime.now(timezone.utc).isoformat(),
        "source_commit": capture(["git", "rev-parse", "HEAD"], env).strip(),
        "dirty_worktree": bool(capture(["git", "status", "--porcelain"], env).strip()),
        "go": capture(["go", "version"], env).strip(),
        "go_environment": capture(["go", "env", "GOOS", "GOARCH", "GOAMD64"], env).splitlines(),
        "platform": platform.platform(),
        "affinity": sorted(os.sched_getaffinity(0)) if hasattr(os, "sched_getaffinity") else None,
        "cpu": next((line.split(":", 1)[1].strip() for line in Path("/proc/cpuinfo").read_text().splitlines()
                     if line.startswith("model name")), "unknown") if Path("/proc/cpuinfo").exists() else platform.processor(),
        "environment": {key: env[key] for key in ("GOTOOLCHAIN", "GOWORK", "GOFLAGS", "GOMAXPROCS")},
        "command": command,
        "go_source_sha256": {path.name: hashlib.sha256(path.read_bytes()).hexdigest()
                             for path in sorted(ROOT.glob("*.go"))},
    }
    (output / "environment.json").write_text(json.dumps(metadata, indent=2) + "\n")
    print("Recording five sequential repeats on", metadata["cpu"], "affinity", metadata["affinity"], flush=True)
    (output / "contracts.txt").write_text(capture(["go", "test", "-count=1", "-run", "^TestReviewExperimentContracts$", "-v", "."], env))
    (output / "benchmarks.txt").write_text(capture(command, env))
    profile = output / "cpu.pprof"
    (output / "profile-run.txt").write_text(capture(["go", "test", "-run", "^$", "-bench",
        "^BenchmarkReview(Arithmetic|Bulk)$", "-benchmem", "-benchtime=1s", "-cpu=1",
        "-cpuprofile", str(profile), "-o", str(output / "profile.test"), "."], env))
    (output / "profile-top.txt").write_text(capture(["go", "tool", "pprof", "-top", str(profile)], env))
    print("Performance evidence recorded at", output, flush=True)

if __name__ == "__main__":
    main()
