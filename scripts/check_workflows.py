"""Check security-critical fields in the repository's formatted workflow layout."""

from pathlib import Path
import re
import sys

ROOT = Path(__file__).resolve().parent.parent


def permissions(text, indent):
    prefix = " " * indent
    headers = re.findall(r"^" + prefix + r"permissions:([^\n]*)\n", text, re.M)
    if not headers:
        return None
    if len(headers) != 1 or headers[0].strip():
        raise ValueError("permissions must be one explicit mapping")
    mapping = re.search(r"^" + prefix + r"permissions:\n((?:" + prefix + r"  [^\n]*\n)+)", text, re.M)
    if not mapping:
        raise ValueError("missing explicit permission entries")
    entries = re.findall(r"^" + prefix + r"  ([a-z-]+): (read|write|none)\s*$", mapping[1], re.M)
    if len(entries) != len(mapping[1].splitlines()) or len(dict(entries)) != len(entries):
        raise ValueError("unrecognized or duplicate permission entries")
    return dict(entries)


def check(workflows):
    for name, text in workflows.items():
        if permissions(text, 0) != {"contents": "read"}:
            raise ValueError(f"{name}: global permissions must be contents: read")
        uses = re.findall(r"^\s*(?:- )?uses:\s*(.*?)\s*$", text, re.M)
        if not uses or any(not re.fullmatch(r"[\w-]+/[\w-]+@[0-9a-f]{40} # v[0-9]+(?:\.[0-9]+)*", entry) for entry in uses):
            raise ValueError(f"{name}: actions need full commit SHAs and same-line version comments")
        if "jobs:\n" not in text:
            raise ValueError(f"{name}: expected an explicit jobs mapping")
        job_text = text.split("jobs:\n", 1)[1]
        jobs = list(re.finditer(r"^  ([\w-]+):\n", job_text, re.M))
        if not jobs:
            raise ValueError(f"{name}: jobs are missing")
        for index, match in enumerate(jobs):
            job = match[1]
            body = job_text[match.end():jobs[index + 1].start() if index + 1 < len(jobs) else len(job_text)]
            if not re.search(r"^    timeout-minutes: [1-9][0-9]*\s*$", body, re.M):
                raise ValueError(f"{name}/{job}: missing job budget")
            allowed = {"pages": "write", "id-token": "write"} if (name, job) == ("wasm-demo-pages.yml", "deploy") else None
            if permissions(body, 4) != allowed:
                raise ValueError(f"{name}/{job}: unexpected job permissions")
            if allowed and not re.search(r"^    needs: build\s*$", body, re.M):
                raise ValueError("Pages deployment must depend on its verified build")
        # Source checkouts do not need credentials left available to later code.
        lines = text.splitlines()
        for index, line in enumerate(lines):
            match = re.match(r"^([ ]*)(- )?uses: actions/checkout@", line)
            if not match:
                continue
            depth = len(match[1]) + (2 if match[2] else 0)
            region = []
            for following in lines[index + 1:]:
                if following.strip() and len(following) - len(following.lstrip()) < depth:
                    break
                region.append(following)
            if not re.search(r"^" + " " * (depth + 2) + r"persist-credentials: false\s*$", "\n".join(region), re.M):
                raise ValueError(f"{name}: checkout must disable persisted credentials")


def load():
    return {path.name: path.read_text() for path in (ROOT / ".github/workflows").glob("*.yml")}


if __name__ == "__main__":
    try:
        check(load())
    except (OSError, ValueError) as error:
        sys.exit(f"error: workflow policy failed: {error}")
    print("Workflow policy passed: SHA/version pins, read-only defaults, deploy-only writes, checkout credentials, and job budgets.")
