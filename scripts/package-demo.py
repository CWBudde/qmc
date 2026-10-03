#!/usr/bin/env python3
"""Archive only verified distribution bytes, without symlinks or hard links."""

import io
import json
from pathlib import Path
import sys
import tarfile

from demo_artifact import MANIFEST, digest, validate

try:
    if len(sys.argv) != 3:
        sys.exit("usage: package-demo.py SITE NEW_ARCHIVE.tar")
    site, output = map(Path, sys.argv[1:])
    if site.resolve() in output.resolve().parents:
        raise ValueError("archive must be outside the verified site")
    manifest = validate(site)
    try:
        # Exclusive creation preserves any caller file already at this path.
        with output.open("xb") as stream, tarfile.open(fileobj=stream, mode="w") as archive:
            for name in sorted(manifest["files"]) + [MANIFEST]:
                path = site / name
                if path.is_symlink() or not path.is_file():
                    raise ValueError(f"artifact entry changed during packaging: {name}")
                data = path.read_bytes()
                if name == MANIFEST:
                    if json.loads(data) != manifest:
                        raise ValueError("manifest changed during packaging")
                elif digest(data) != manifest["files"][name]:
                    raise ValueError(f"artifact changed during packaging: {name}")
                entry = tarfile.TarInfo(name)
                entry.size, entry.mode, entry.mtime = len(data), 0o644, 0
                archive.addfile(entry, io.BytesIO(data))
        if output.stat().st_size >= 10_000_000_000:
            raise ValueError("Pages archive must be smaller than 10 GB")
    except FileExistsError:
        raise ValueError("archive destination already exists; caller file preserved") from None
    except BaseException:
        output.unlink(missing_ok=True)
        raise
    print(f"Packaged {len(manifest['files']) + 1} verified regular files into {output}")
except (OSError, ValueError, KeyError, TypeError, tarfile.TarError) as error:
    sys.exit(f"error: demo packaging failed: {error}")
