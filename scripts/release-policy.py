#!/usr/bin/env python3
"""Validate release policy without modifying source or creating a tag."""

import os
from pathlib import Path
import subprocess
import sys

from release_common import metadata, normalize_version, source, workflow_version

try:
    if sys.argv[1:] == ["--workflow"]:
        version, reviewed = workflow_version()
        with Path(os.environ["GITHUB_OUTPUT"]).open("a") as output:
            output.write(f"version={version}\nreviewed_commit={reviewed}\n")
        print(f"Validating v{version} from reviewed commit {reviewed}")
    elif len(sys.argv) == 3:
        version = normalize_version(sys.argv[1])
        source(sys.argv[2])
        metadata(version)
        print(version)
    else:
        sys.exit("usage: release-policy.py VERSION REVIEWED_COMMIT | --workflow")
except (OSError, ValueError, KeyError, subprocess.SubprocessError) as error:
    sys.exit(f"error: release policy failed: {error}")
