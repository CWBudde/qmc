#!/usr/bin/env python3
"""Prove module checks reject drift/defects without changing source metadata."""

import os
from pathlib import Path
import shutil
import subprocess
import tempfile

ROOT = Path(__file__).resolve().parent.parent

with tempfile.TemporaryDirectory(prefix="qmc-module-gates-") as directory:
    copy = Path(directory)
    for source in ROOT.glob("*.go"):
        shutil.copy(source, copy / source.name)
    for name in ["go.mod", "justfile"]:
        shutil.copy(ROOT / name, copy / name)
    shutil.copytree(ROOT / "third_party", copy / "third_party")
    demo = copy / "examples/wasm-demo"
    demo.mkdir(parents=True)
    for source in (ROOT / "examples/wasm-demo").iterdir():
        if source.suffix == ".go" or source.name == "go.mod":
            shutil.copy(source, demo / source.name)
    # This synthetic source copy has no VCS history; production keeps stamping.
    environment = {**os.environ, "GOFLAGS": "-buildvcs=false", "GOPROXY": "off"}

    def run(recipe):
        return subprocess.run(
            ["just", recipe], cwd=copy, env=environment,
            capture_output=True, text=True, timeout=90,
        )

    for module in [copy, demo]:
        path = module / "go.mod"
        original = path.read_bytes()
        path.write_bytes(original + b"\nrequire example.invalid/unused v0.0.0\n")
        untidy = path.read_bytes()
        result = run("check-tidy")
        assert result.returncode != 0, "untidy module passed: " + str(module)
        assert "diff current/go.mod tidy/go.mod" in result.stdout, result.stdout + result.stderr
        assert path.read_bytes() == untidy, "check-tidy rewrote metadata: " + str(module)
        assert not (module / "go.sum").exists(), "check-tidy created a sum file"
        path.write_bytes(original)

    probe = demo / "vet_probe.go"
    probe.write_text(
        '//go:build js && wasm\n\npackage main\n\nimport "fmt"\n\n'
        'func vetProbe() { fmt.Printf("%d", "wrong") }\n'
    )
    result = run("check-wasm-demo")
    assert result.returncode != 0 and "fmt.Printf format %d" in result.stderr, result.stdout + result.stderr

print("Module gates passed: root/demo tidy drift fails without writes; compiling WASM printf defect fails vet.")
