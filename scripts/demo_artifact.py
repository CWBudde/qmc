"""Integrity and static-reference checks for a complete QMC demo distribution."""

import hashlib
from html.parser import HTMLParser
import json
from pathlib import Path, PurePosixPath
import re
from urllib.parse import unquote, urljoin, urlsplit

MANIFEST = "build-manifest.json"
PRODUCER = "qmc-wasm-demo"
NOTICES = {"notices/qmc-LICENSE.txt", "notices/joe-kuo-LICENSE.txt",
           "notices/go-LICENSE.txt", "notices/go-PATENTS.txt"}
PAGES = {"index.html", "analysis.html", "credits.html"}
RUNTIME_REQUIRED = {"index.html", "analysis.html", "qmc.wasm", "wasm_exec.js", "compute-worker.js", "compute.js"}
REQUIRED = PAGES | NOTICES | RUNTIME_REQUIRED
HEX = re.compile(r"[a-f0-9]{64}\Z")


def digest(data):
    return hashlib.sha256(data).hexdigest()


def build_id(inputs, toolchain):
    return digest(json.dumps({"inputs": inputs, "toolchain": toolchain}, sort_keys=True, separators=(",", ":")).encode())


def safe_name(name):
    if not isinstance(name, str) or not name or "\\" in name:
        raise ValueError("invalid artifact path")
    path = PurePosixPath(name)
    if path.is_absolute() or any(part in (".", "..") for part in name.split("/")) or "" in name.split("/"):
        raise ValueError(f"invalid artifact path: {name}")
    return path


def inventory(root):
    files, directories = set(), set()
    for path in root.rglob("*"):
        relative = path.relative_to(root).as_posix()
        if path.is_symlink():
            raise ValueError(f"symlink in artifact: {relative}")
        if path.is_file():
            files.add(relative)
        elif path.is_dir():
            directories.add(relative)
        else:
            raise ValueError(f"non-regular artifact entry: {relative}")
    return files, directories


def public_entry(data, identifier, pages):
    """Keep navigation at stable public pages; resolve resources within one build."""
    source = data.decode("utf-8")
    if re.search(r"<base\b", source, re.I):
        raise ValueError("source entry pages must not set their own base URL")

    def navigation(match):
        url = urlsplit(match[3])
        logical = url.path.removeprefix("./")
        if url.scheme or url.netloc or logical not in pages:
            return match[0]
        suffix = ("?" + url.query if url.query else "") + ("#" + url.fragment if url.fragment else "")
        return f"{match[1]}{match[2]}../{logical}{suffix}{match[2]}"

    source = re.sub(r"(\bhref\s*=\s*)([\"'])([^\"']+)\2", navigation, source, flags=re.I)
    base = f'\n    <base href="./build-{identifier}/" />'
    source, count = re.subn(r"<head(?:\s[^>]*)?>", lambda match: match[0] + base, source, count=1, flags=re.I)
    if count != 1:
        raise ValueError("entry page needs a head element")
    return source.encode()


def anchor_paths(data):
    class Anchors(HTMLParser):
        paths = None

        def __init__(self):
            super().__init__()
            self.paths = set()

        def handle_starttag(self, tag, attributes):
            href = dict(attributes).get("href", "")
            url = urlsplit(href)
            if tag == "a" and not url.scheme and not url.netloc:
                self.paths.add(url.path.removeprefix("./"))

    parser = Anchors()
    parser.feed(data.decode("utf-8"))
    return parser.paths


def validate(root, references=True, distribution=True):
    root = Path(root)
    if root.is_symlink() or not root.is_dir():
        raise ValueError("artifact root must be a real directory")
    if (root / MANIFEST).is_symlink() or not (root / MANIFEST).is_file():
        raise ValueError("artifact manifest must be a regular file")
    manifest = json.loads((root / MANIFEST).read_text())
    if manifest.get("producer") != PRODUCER or manifest.get("schema") != 1:
        raise ValueError("unrecognized demo ownership manifest")
    inputs, files, pages = manifest["inputs"], manifest["files"], manifest["pages"]
    if not isinstance(inputs, dict) or not isinstance(files, dict) or not isinstance(pages, list):
        raise ValueError("invalid artifact manifest structure")
    required = REQUIRED if distribution else RUNTIME_REQUIRED
    required_pages = PAGES if distribution else {"index.html", "analysis.html"}
    if not required <= inputs.keys() or not required_pages <= set(pages):
        raise ValueError("artifact lacks required pages/runtime/worker assets/notices")
    for name, value in {**inputs, **files}.items():
        safe_name(name)
        if not isinstance(value, str) or not HEX.fullmatch(value):
            raise ValueError(f"invalid hash for {name}")
    toolchain = manifest.get("toolchain")
    if not isinstance(toolchain, str) or not re.fullmatch(r"go\d+\.\d+(?:\.\d+)?", toolchain):
        raise ValueError("invalid compiler version in artifact")
    identifier = build_id(inputs, toolchain)
    if identifier != manifest.get("build_id"):
        raise ValueError("build identifier disagrees with content manifest")
    prefix = f"build-{identifier}/"
    expected = {prefix + name for name in inputs} | set(pages)
    if set(files) != expected or len(set(pages)) != len(pages):
        raise ValueError("unexpected or missing manifest file entries")
    if any(page not in inputs or "/" in page or not page.endswith(".html") for page in pages):
        raise ValueError("invalid public entry page")
    actual, directories = inventory(root)
    expected_dirs = {str(parent) for name in expected for parent in PurePosixPath(name).parents if str(parent) != "."}
    if actual != expected | {MANIFEST} or directories != expected_dirs:
        raise ValueError("artifact has missing or caller-added files/directories")
    for name, expected_hash in files.items():
        if digest((root / name).read_bytes()) != expected_hash:
            raise ValueError(f"artifact content changed: {name}")
    for logical, source_hash in inputs.items():
        if files[prefix + logical] != source_hash:
            raise ValueError(f"bundle hash differs from input: {logical}")
    for page in pages:
        expected_page = public_entry((root / prefix / page).read_bytes(), identifier, pages)
        if (root / page).read_bytes() != expected_page:
            raise ValueError(f"entry page does not select its complete build: {page}")
    if distribution:
        for notice in NOTICES:
            if not (root / prefix / notice).read_text().strip():
                raise ValueError(f"empty distribution notice: {notice}")
        for page in ("index.html", "analysis.html"):
            if "credits.html" not in anchor_paths((root / prefix / page).read_bytes()):
                raise ValueError(f"credits are unreachable from {page}")
        if not NOTICES <= anchor_paths((root / prefix / "credits.html").read_bytes()):
            raise ValueError("credits page must link every required distribution notice")
    if not (root / prefix / "qmc.wasm").read_bytes().startswith(b"\0asm\x01\0\0\0"):
        raise ValueError("invalid WASM header")
    if references:
        check_references(root, manifest)
    return manifest


def check_references(root, manifest):
    available = set(manifest["files"])
    origin = "https://qmc.invalid/"

    def check(base, value, resource):
        if not value or value.startswith("#") or value.startswith("data:"):
            return
        target = urlsplit(urljoin(base, value))
        if target.scheme != "https" or target.netloc != "qmc.invalid":
            if resource:
                raise ValueError(f"external resource in artifact: {value}")
            return
        name = unquote(target.path.lstrip("/"))
        if name not in available:
            raise ValueError(f"unresolved artifact reference: {value} from {base}")

    class Links(HTMLParser):
        def __init__(self, base):
            super().__init__()
            self.base = base

        def handle_starttag(self, tag, attributes):
            attrs = dict(attributes)
            if tag == "base":
                self.base = urljoin(self.base, attrs.get("href", ""))
                return
            for key in ["src", "poster", "href", "xlink:href"]:
                if key in attrs:
                    check(self.base, attrs[key], key != "href" or tag != "a")
            if "srcset" in attrs:
                for candidate in attrs["srcset"].split(","):
                    check(self.base, candidate.strip().split()[0], True)

    for name in available:
        path = root / name
        base = urljoin(origin, name)
        suffix = path.suffix.lower()
        if suffix in (".html", ".svg"):
            Links(base).feed(path.read_text())
        elif suffix == ".css":
            for match in re.finditer(r"url\(\s*([\"']?)(.*?)\1\s*\)", path.read_text()):
                check(base, match[2], True)
            for match in re.finditer(r"@import\s+[\"']([^\"']+)[\"']", path.read_text()):
                check(base, match[1], True)
        elif suffix in (".js", ".mjs"):
            source = path.read_text()
            for match in re.finditer(r"(?:fetch|new\s+Worker|new\s+URL)\s*\(\s*[\"']([^\"']+)[\"']", source):
                check(base, match[1], True)
            for match in re.finditer(r"importScripts\(([^)]*)\)", source):
                for url in re.findall(r"[\"']([^\"']+)[\"']", match[1]):
                    check(base, url, True)
            for match in re.finditer(r"(?:\bfrom|\bimport)\s*[\"']([^\"']+)[\"']", source):
                check(base, match[1], True)
