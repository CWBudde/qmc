#!/usr/bin/env python3
"""Check the exact manifest, content hashes, and static references before upload."""
from pathlib import Path
import sys

from demo_artifact import validate

try:
    manifest = validate(Path(sys.argv[1] if len(sys.argv) > 1 else "dist"))
except (OSError, ValueError, KeyError, TypeError) as error:
    sys.exit(f"error: invalid demo artifact: {error}")
print(f"Demo artifact verified: {manifest['build_id']} ({len(manifest['files'])} files).")
