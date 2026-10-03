#!/usr/bin/env python3
"""Exercise artifact publication with real files/renames and a fake Go compiler."""

import importlib.util
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import threading
from concurrent.futures import ThreadPoolExecutor

from demo_artifact import MANIFEST, digest, validate

ROOT = Path(__file__).resolve().parent.parent

with tempfile.TemporaryDirectory(prefix="qmc-artifact-test-") as directory:
    temporary = Path(directory)
    copy = temporary / "source"
    scripts = copy / "scripts"
    scripts.mkdir(parents=True)
    for name in ["build-wasm-demo.sh", "build-wasm-demo.py", "demo_artifact.py"]:
        shutil.copy(ROOT / "scripts" / name, scripts / name)
    demo = copy / "examples/wasm-demo"
    demo.mkdir(parents=True)
    for source in (ROOT / "examples/wasm-demo").iterdir():
        if source.suffix in {".html", ".css", ".js", ".mjs", ".svg"}:
            shutil.copy(source, demo / source.name)
    assets = demo / "assets/nested"
    assets.mkdir(parents=True)
    for name, data in [("points.json", b'{"point": 1}'), ("image.png", b"PNG"), ("font.woff2", b"font")]:
        (assets / name).write_bytes(data)
    (demo / "style.css").write_text((demo / "style.css").read_text() +
                                    '\n.probe { background: url("assets/nested/image.png"); }\n')
    toolchain = temporary / "compiler/lib/wasm"
    toolchain.mkdir(parents=True)
    (toolchain / "wasm_exec.js").write_text("// compiler-matched fixture runtime\n")
    mock = temporary / "bin"
    mock.mkdir()
    go = mock / "go"
    go.write_text(f"#!{sys.executable}\n" + '''
import json, os, pathlib, sys
args=sys.argv[1:]
if 'env' in args:
    version='go1.25.0' if os.environ.get('QMC_TEST_COMPILER_CHANGED') and pathlib.Path(os.environ['QMC_TEST_COMPILER_CHANGED']).exists() else 'go1.26.1'
    print(json.dumps({'GOROOT':os.environ['QMC_TEST_GOROOT'],'GOVERSION':version}))
else:
    if os.environ.get('QMC_TEST_GO_FAIL'):sys.exit(42)
    if os.environ.get('QMC_TEST_CALLER_EDIT'):
        pathlib.Path(os.environ['QMC_TEST_CALLER_EDIT']).write_text('caller note')
    pathlib.Path(args[args.index('-o')+1]).write_bytes(b'\\0asm\\x01\\0\\0\\0fixture')
    if os.environ.get('QMC_TEST_COMPILER_CHANGED'):
        pathlib.Path(os.environ['QMC_TEST_COMPILER_CHANGED']).touch()
''')
    go.chmod(0o755)
    environment = {**os.environ, "PATH": str(mock) + os.pathsep + os.environ["PATH"],
                   "QMC_TEST_GOROOT": str(toolchain.parent.parent)}
    output = temporary / "build with spaces"

    def run(destination=output, **extra):
        return subprocess.run(
            ["bash", str(scripts / "build-wasm-demo.sh"), str(destination)],
            cwd=temporary, env={**environment, **extra}, capture_output=True,
            text=True, timeout=30,
        )

    def success(result):
        assert result.returncode == 0, result.stdout + result.stderr

    with ThreadPoolExecutor(max_workers=4) as pool:
        for result in pool.map(lambda _: run(), range(4)):
            success(result)
    first = validate(output)
    assert "assets/nested/points.json" in first["inputs"]
    assert "assets/nested/font.woff2" in first["inputs"]
    old_prefix = "build-" + first["build_id"]
    assert (output / old_prefix / "wasm_exec.js").read_bytes() == (toolchain / "wasm_exec.js").read_bytes()
    success(run())
    assert validate(output)["build_id"] == first["build_id"], "identical inputs changed identity"
    with ThreadPoolExecutor(max_workers=4) as pool:
        for result in pool.map(lambda _: run(), range(4)):
            success(result)
    assert validate(output)["build_id"] == first["build_id"], "concurrent builds changed identity"
    lock = temporary / (".qmc-demo-" + digest(os.fsencode(output)) + ".lock")
    marker = lock.read_bytes()
    lock.write_bytes(b"caller lock content")
    assert run().returncode != 0 and lock.read_bytes() == b"caller lock content"
    lock.write_bytes(marker)
    for page in ["index.html", "analysis.html"]:
        assert f'<base href="./{old_prefix}/"' in (output / page).read_text()
        assert 'href="../index.html"' in (output / page).read_text()
    (assets / "points.json").unlink()
    success(run())
    second = validate(output)
    assert second["build_id"] != first["build_id"] and not (output / old_prefix).exists()
    assert "assets/nested/points.json" not in second["inputs"]
    original = (output / MANIFEST).read_bytes()
    failure = run(QMC_TEST_GO_FAIL="1")
    assert failure.returncode != 0 and (output / MANIFEST).read_bytes() == original
    validate(output)
    compiler_changed = temporary / "compiler-changed"
    assert run(QMC_TEST_COMPILER_CHANGED=str(compiler_changed)).returncode != 0
    assert (output / MANIFEST).read_bytes() == original
    runtime = toolchain / "wasm_exec.js"
    runtime_bytes = runtime.read_bytes()
    runtime.unlink()
    assert run().returncode != 0 and (output / MANIFEST).read_bytes() == original
    runtime.write_bytes(runtime_bytes)
    note = output / "caller-note.txt"
    note.write_text("preserve")
    assert run().returncode != 0 and note.read_text() == "preserve"
    note.unlink()
    changed = output / "index.html"
    saved = changed.read_bytes()
    changed.write_bytes(saved + b"caller edit")
    assert run().returncode != 0 and changed.read_bytes() == saved + b"caller edit"
    changed.write_bytes(saved)
    failure = run(QMC_TEST_CALLER_EDIT=str(note))
    assert failure.returncode != 0 and note.read_text() == "caller note"
    assert (output / MANIFEST).read_bytes() == original
    note.unlink()
    unrelated = temporary / "unowned"
    unrelated.mkdir()
    (unrelated / "notes.txt").write_text("preserve")
    assert run(unrelated).returncode != 0 and (unrelated / "notes.txt").read_text() == "preserve"
    link = temporary / "output-link"
    link.symlink_to(output, target_is_directory=True)
    assert run(link).returncode != 0 and link.is_symlink()
    assert run(copy).returncode != 0 and run(demo / "output").returncode != 0
    asset_link = demo / "assets/link.txt"
    asset_link.symlink_to(unrelated / "notes.txt")
    assert run().returncode != 0
    asset_link.unlink()
    moved_assets = demo / "assets.original"
    (demo / "assets").rename(moved_assets)
    (demo / "assets").symlink_to(moved_assets, target_is_directory=True)
    assert run().returncode != 0, "top-level asset symlink was followed"
    (demo / "assets").unlink()
    moved_assets.rename(demo / "assets")
    fifo = assets / "stream.txt"
    os.mkfifo(fifo)
    assert run().returncode != 0, "non-regular recursive asset was silently skipped"
    fifo.unlink()
    (demo / "assets").rename(moved_assets)
    (demo / "assets").write_text("not a directory")
    assert run().returncode != 0, "non-directory assets source was silently skipped"
    (demo / "assets").unlink()
    moved_assets.rename(demo / "assets")
    unsupported = assets / "secret.go"
    unsupported.write_text("package private")
    assert run().returncode != 0
    unsupported.unlink()
    (demo / "style.css").write_text((demo / "style.css").read_text() + '\n.x { background: url("missing.png"); }\n')
    assert run().returncode != 0 and (output / MANIFEST).read_bytes() == original
    (demo / "style.css").write_text((demo / "style.css").read_text().removesuffix('\n.x { background: url("missing.png"); }\n'))
    # Standalone validator rejects edited bytes and unexpected empty directories.
    (output / "unexpected-directory").mkdir()
    try:
        validate(output)
    except ValueError:
        pass
    else:
        raise AssertionError("caller-added directory was accepted")
    (output / "unexpected-directory").rmdir()
    validate(output)
    assert not list(temporary.glob(".qmc-demo-stage-*")), "failed staging trees leaked"

    # Exercise the native exchange operation under readers without cleanup races.
    spec = importlib.util.spec_from_file_location("builder", ROOT / "scripts/build-wasm-demo.py")
    builder = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(builder)
    left, right = temporary / "left", temporary / "right"
    left.mkdir(); right.mkdir()
    (left / "entry").write_text("old"); (right / "entry").write_text("new")
    failures, done = [], threading.Event()

    def read_entries():
        while not done.is_set():
            try:
                assert (left / "entry").read_text() in {"old", "new"}
            except Exception as error:
                failures.append(str(error))

    reader = threading.Thread(target=read_entries)
    reader.start()
    try:
        for _ in range(500):
            builder.exchange(left, right)
    finally:
        done.set(); reader.join()
    assert not failures, failures

    # A caller edit racing the exchange must survive cleanup at its private
    # preceding-tree path. This is distinct from edits caught before the swap.
    mutation = subprocess.run([sys.executable, "-c", '''
import importlib.util, pathlib, sys
sys.path.insert(0,str(pathlib.Path(sys.argv[1]).parent))
spec=importlib.util.spec_from_file_location('builder',sys.argv[1])
builder=importlib.util.module_from_spec(spec)
spec.loader.exec_module(builder)
exchange=builder.exchange
def edited_exchange(stage,output):
    exchange(stage,output)
    (stage/'caller-note.txt').write_text('racing edit')
builder.exchange=edited_exchange
builder.build(pathlib.Path(sys.argv[2]))
''', str(scripts / "build-wasm-demo.py"), str(output)],
        env=environment, capture_output=True, text=True, timeout=30)
    assert mutation.returncode != 0 and "edited preceding output preserved" in mutation.stderr, mutation.stderr
    preserved = list(temporary.glob(".qmc-demo-stage-*"))
    assert len(preserved) == 1 and (preserved[0] / "caller-note.txt").read_text() == "racing edit"
    validate(output)

print("Demo artifact gates passed: content identity, recursive assets, removed assets, atomic exchange, caller preservation before/after publication, failures, paths, and references.")
