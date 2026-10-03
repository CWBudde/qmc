#!/usr/bin/env python3
"""Build and atomically publish a complete, content-addressed static demo."""

import ctypes
import fcntl
import json
import os
from pathlib import Path
import re
import shutil
import stat
import subprocess
import sys
import tempfile
import time

from demo_artifact import MANIFEST, PRODUCER, build_id, digest, public_entry, validate

ROOT = Path(__file__).resolve().parent.parent
DEMO = ROOT / "examples/wasm-demo"
STATIC = {".html", ".css", ".js", ".mjs", ".svg"}
ASSETS = STATIC | {".png", ".jpg", ".jpeg", ".gif", ".webp", ".avif", ".ico", ".json",
                   ".woff", ".woff2", ".ttf", ".otf", ".txt", ".pdf"}


def destination(argument):
    path = Path(argument).expanduser().absolute()
    if path.is_symlink():
        raise ValueError("output destination cannot be a symlink")
    path = path.resolve()
    if path in (Path("/"), Path.home(), ROOT) or path in ROOT.parents:
        raise ValueError("output destination is a protected directory")
    for protected in [DEMO, ROOT / ".git", ROOT / ".agents", ROOT / ".codex", ROOT / "third_party"]:
        if path == protected or protected in path.parents:
            raise ValueError("output destination overlaps source or private state")
    return path


def verify_destination(path):
    if path.is_symlink():
        raise ValueError("output destination became a symlink")
    if not path.exists():
        return
    if not path.is_dir() or path.stat().st_uid != os.getuid():
        raise ValueError("output must be a user-owned directory")
    if any(path.iterdir()):
        try:
            validate(path, distribution=False)
        except (OSError, ValueError, KeyError, TypeError) as error:
            raise ValueError(f"existing output is unowned or edited; preserved at {path}: {error}") from error


def exchange(stage, output):
    libc = ctypes.CDLL(None, use_errno=True)
    if sys.platform == "linux" and hasattr(libc, "renameat2"):
        operation = libc.renameat2
        operation.argtypes = [ctypes.c_int, ctypes.c_char_p, ctypes.c_int, ctypes.c_char_p, ctypes.c_uint]
        operation.restype = ctypes.c_int
        result = operation(-100, os.fsencode(stage), -100, os.fsencode(output), 2)  # RENAME_EXCHANGE
    elif sys.platform == "darwin" and hasattr(libc, "renamex_np"):
        operation = libc.renamex_np
        operation.argtypes = [ctypes.c_char_p, ctypes.c_char_p, ctypes.c_uint]
        operation.restype = ctypes.c_int
        result = operation(os.fsencode(stage), os.fsencode(output), 2)  # RENAME_SWAP
    else:
        raise ValueError("atomic replacement is unavailable; choose a fresh output directory")
    if result:
        raise OSError(ctypes.get_errno(), "atomic directory replacement failed; previous output preserved")


def build(output):
    output.parent.mkdir(parents=True, exist_ok=True)
    lock = output.parent / (".qmc-demo-" + digest(os.fsencode(output)) + ".lock")
    try:
        descriptor = os.open(lock, os.O_RDWR | os.O_NOFOLLOW)
    except FileNotFoundError:
        # Publish an already initialized inode. A second fresh builder must
        # never observe an empty lock marker between creation and writing.
        descriptor, temporary = tempfile.mkstemp(prefix=".qmc-demo-initial-", suffix=".lock", dir=output.parent)
        try:
            os.write(descriptor, b"qmc-wasm-demo-lock\n")
            try:
                os.link(temporary, lock)
            except FileExistsError:
                os.close(descriptor)
                descriptor = os.open(lock, os.O_RDWR | os.O_NOFOLLOW)
        except BaseException:
            try:
                os.close(descriptor)
            except OSError:
                pass
            raise
        finally:
            os.unlink(temporary)
    try:
        information = os.fstat(descriptor)
        if not stat.S_ISREG(information.st_mode) or information.st_uid != os.getuid():
            raise ValueError("build lock must be a user-owned regular file")
        deadline = time.monotonic() + 30
        while True:
            try:
                fcntl.flock(descriptor, fcntl.LOCK_EX | fcntl.LOCK_NB)
                break
            except BlockingIOError:
                if time.monotonic() >= deadline:
                    raise ValueError("another build held the output lock for thirty seconds")
                time.sleep(0.05)
        os.lseek(descriptor, 0, os.SEEK_SET)
        if os.read(descriptor, 64) != b"qmc-wasm-demo-lock\n":
            raise ValueError("unrecognized build lock; caller file preserved")
        verify_destination(output)
        stage = Path(tempfile.mkdtemp(prefix=".qmc-demo-stage-", dir=output.parent))
        discard_stage = True
        try:
            environment = {**os.environ, "GOOS": "js", "GOARCH": "wasm"}
            compiler = json.loads(subprocess.check_output(
                ["go", "-C", str(DEMO), "env", "-json", "GOROOT", "GOVERSION"], env=environment, timeout=180))
            toolchain = Path(compiler["GOROOT"])
            runtime = next((path for path in [toolchain / "lib/wasm/wasm_exec.js",
                                             toolchain / "misc/wasm/wasm_exec.js"] if path.is_file()), None)
            if runtime is None:
                raise ValueError("compiler-matched wasm_exec.js is missing")
            for path in DEMO.iterdir():
                if path.suffix in STATIC and (path.is_symlink() or not path.is_file()):
                    raise ValueError("static source assets must be regular nonsymlink files")
            payload = {path.name: path.read_bytes() for path in DEMO.iterdir()
                       if path.is_file() and path.suffix in STATIC}
            asset_dir = DEMO / "assets"
            if asset_dir.is_symlink():
                raise ValueError("assets directory cannot be a symlink")
            if asset_dir.exists():
                if not asset_dir.is_dir():
                    raise ValueError("assets must be a directory")
                for path in asset_dir.rglob("*"):
                    if path.is_symlink():
                        raise ValueError("recursive assets cannot be symlinks")
                    if path.is_file():
                        if path.suffix.lower() not in ASSETS:
                            raise ValueError(f"unrecognized static asset type: {path}")
                        payload[path.relative_to(DEMO).as_posix()] = path.read_bytes()
                    elif not path.is_dir():
                        raise ValueError(f"non-regular static asset: {path}")
            subprocess.run(["go", "-C", str(DEMO), "build", "-trimpath", "-o", str(stage / "qmc.wasm"), "."],
                           env=environment, timeout=180, check=True)
            current = json.loads(subprocess.check_output(
                ["go", "-C", str(DEMO), "env", "-json", "GOROOT", "GOVERSION"], env=environment, timeout=180))
            if current != compiler:
                raise ValueError("Go toolchain changed while building")
            payload["qmc.wasm"] = (stage / "qmc.wasm").read_bytes()
            (stage / "qmc.wasm").unlink()
            payload["wasm_exec.js"] = runtime.read_bytes()
            notices = {"notices/qmc-LICENSE.txt": ROOT / "LICENSE",
                       "notices/joe-kuo-LICENSE.txt": ROOT / "third_party/joe-kuo/LICENSE.txt",
                       "notices/go-LICENSE.txt": toolchain / "LICENSE",
                       "notices/go-PATENTS.txt": toolchain / "PATENTS"}
            for name, path in notices.items():
                if path.is_symlink() or not path.is_file():
                    raise ValueError(f"required notice must be a regular file: {path}")
                payload[name] = path.read_bytes()
            inputs = {name: digest(data) for name, data in payload.items()}
            identifier = build_id(inputs, compiler["GOVERSION"])
            prefix = f"build-{identifier}"
            pages = sorted(name for name in payload if "/" not in name and name.endswith(".html"))
            files = {}
            for name, data in payload.items():
                destination = stage / prefix / name
                destination.parent.mkdir(parents=True, exist_ok=True)
                destination.write_bytes(data)
                files[prefix + "/" + name] = digest(data)
            for page in pages:
                data = public_entry(payload[page], identifier, pages)
                (stage / page).write_bytes(data)
                files[page] = digest(data)
            manifest = {"producer": PRODUCER, "schema": 1, "build_id": identifier,
                        "toolchain": compiler["GOVERSION"], "inputs": inputs,
                        "pages": pages, "files": files}
            (stage / MANIFEST).write_text(json.dumps(manifest, indent=2, sort_keys=True) + "\n")
            for path in [stage, *stage.rglob("*")]:
                path.chmod(0o755 if path.is_dir() else 0o644)
            validate(stage)
            verify_destination(output)  # Detect caller edits made during compilation.
            if output.exists() and any(output.iterdir()):
                exchange(stage, output)
                # The preceding tree now has a private name. Recheck it before
                # cleanup so an edit racing the swap is retained, not deleted.
                discard_stage = False
                try:
                    validate(stage, distribution=False)
                except (OSError, ValueError, KeyError, TypeError) as error:
                    raise ValueError(f"new build published; edited preceding output preserved at {stage}: {error}") from error
                discard_stage = True
            else:
                os.replace(stage, output)
            print(f"WASM demo built at {output}\nBuild {identifier}: {len(files)} verified files")
        finally:
            # After exchange this name owns the fully verified preceding build.
            if discard_stage and stage.exists():
                shutil.rmtree(stage)
    finally:
        os.close(descriptor)


if __name__ == "__main__":
    try:
        build(destination(sys.argv[1] if len(sys.argv) > 1 else ROOT / "dist"))
    except (OSError, ValueError, KeyError, TypeError, subprocess.SubprocessError) as error:
        sys.exit(f"error: demo build failed: {error}")
