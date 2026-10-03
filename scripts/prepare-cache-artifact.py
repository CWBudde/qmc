#!/usr/bin/env python3
"""Create a second coherent browser-test bundle without recompiling its WASM."""

import json
from pathlib import Path
import sys

from demo_artifact import MANIFEST, build_id, digest, public_entry, validate

source, output = map(Path, sys.argv[1:])
previous = validate(source)
if not output.is_dir() or any(output.iterdir()):
    sys.exit("cache fixture needs a fresh empty directory")
payload = {name: (source / f'build-{previous["build_id"]}' / name).read_bytes()
           for name in previous["inputs"]}
payload["style.css"] += b"\n/* second coherent browser-test build */\n"
inputs = {name: digest(data) for name, data in payload.items()}
identifier = build_id(inputs, previous["toolchain"])
prefix = f"build-{identifier}/"
files = {}
for name, data in payload.items():
    path = output / prefix / name
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_bytes(data)
    files[prefix + name] = digest(data)
for page in previous["pages"]:
    data = public_entry(payload[page], identifier, previous["pages"])
    (output / page).write_bytes(data)
    files[page] = digest(data)
(output / MANIFEST).write_text(json.dumps({**previous, "build_id": identifier,
                                         "inputs": inputs, "files": files}, indent=2, sort_keys=True) + "\n")
validate(output)
print(identifier)
