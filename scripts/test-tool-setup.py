#!/usr/bin/env python3
"""Offline installer regressions: real archives/checksums, mocked downloads/builds."""

import hashlib
import io
import json
import os
from pathlib import Path
import shutil
import subprocess
import tarfile
import tempfile

ROOT = Path(__file__).resolve().parent.parent
VERSIONS = {
    "treefmt": "2.5.0", "shellcheck": "0.11.0", "gofumpt": "0.10.0",
    "gci": "0.14.0", "shfmt": "3.12.0", "golangci-lint": "2.13.1",
    "prettier": "3.5.3",
}


def binary(name, version):
    prefixes = {"treefmt": "treefmt v", "shellcheck": "version: ",
                "gofumpt": "v", "gci": "gci version ", "shfmt": "v",
                "golangci-lint": "golangci-lint has version "}
    return f"#!/bin/bash\nprintf '%s\\n' '{prefixes.get(name, '')}{version}'\n".encode()


def executable(path, content):
    path.write_bytes(content if isinstance(content, bytes) else content.encode())
    path.chmod(0o755)


with tempfile.TemporaryDirectory(prefix="qmc-setup-test-") as temporary:
    root = Path(temporary)
    (root / "scripts").mkdir()
    (root / "tools").mkdir()
    shutil.copy(ROOT / "scripts/setup-deps.sh", root / "scripts")
    for name in ["versions.sh", "go-version", "package.json", "package-lock.json"]:
        shutil.copy(ROOT / "tools" / name, root / "tools")
    assets = root / "assets"
    assets.mkdir()
    pins = []
    platforms = [
        ("Linux", "x86_64", "linux_amd64", "linux.x86_64"),
        ("Linux", "aarch64", "linux_arm64", "linux.aarch64"),
        ("Darwin", "x86_64", "darwin_amd64", "darwin.x86_64"),
        ("Darwin", "arm64", "darwin_arm64", "darwin.aarch64"),
    ]
    for _, _, tree_platform, shell_platform in platforms:
        for name, asset, member in [
            ("treefmt", f"treefmt_2.5.0_{tree_platform}.tar.gz", "treefmt"),
            ("shellcheck", f"shellcheck-v0.11.0.{shell_platform}.tar.gz", "shellcheck-v0.11.0/shellcheck"),
        ]:
            content = binary(name, VERSIONS[name])
            with tarfile.open(assets / asset, "w:gz") as archive:
                info = tarfile.TarInfo(member)
                info.size, info.mode = len(content), 0o755
                archive.addfile(info, io.BytesIO(content))
            pins.append(f"{hashlib.sha256((assets / asset).read_bytes()).hexdigest()}  {asset}")
    (root / "tools/archive-checksums.sha256").write_text("\n".join(pins) + "\n")
    mock = root / "mock"
    mock.mkdir()
    executable(mock / "uname", "#!/bin/bash\nif [[ $1 == -s ]]; then echo \"$QMC_TEST_OS\"; else echo \"$QMC_TEST_ARCH\"; fi\n")
    mock_driver = f"#!{shutil.which('python3')}\n" + '''
import json, os, pathlib, shutil, subprocess, sys
name=pathlib.Path(sys.argv[0]).name
args=sys.argv[1:]
with open(os.environ['QMC_TEST_LOG'],'a') as log:
    log.write(json.dumps({'name':name,'args':args,'sumdb':os.environ.get('GOSUMDB'),'nosumdb':os.environ.get('GONOSUMDB')})+'\\n')
if name=='curl':
    asset=args[args.index('-o')-1].split('/')[-1]
    destination=pathlib.Path(args[args.index('-o')+1])
    shutil.copy(pathlib.Path(os.environ['QMC_TEST_ASSETS'])/asset,destination)
    if os.environ.get('QMC_TEST_CORRUPT'): destination.write_bytes(destination.read_bytes()+b'corruption')
elif name=='tar':
    sys.exit(subprocess.call([os.environ['QMC_TEST_TAR'],*args]))
elif name=='go':
    module=args[-1]
    tool=module.split('@')[0].split('/')[-1]
    version=module.split('@v')[1]
    prefix={'gofumpt':'v','gci':'gci version ','shfmt':'v','golangci-lint':'golangci-lint has version '}[tool]
    if os.environ.get('QMC_TEST_WRONG_GO'): version='0.0.0'
    output=pathlib.Path(os.environ['GOBIN'])/tool
    output.write_text('#!/bin/bash\\necho "'+prefix+version+'"\\n');output.chmod(0o755)
elif name=='npm':
    if os.environ.get('QMC_TEST_NPM_FAIL'): sys.exit(42)
    prefix=pathlib.Path(args[args.index('--prefix')+1])
    assert json.loads((prefix/'package-lock.json').read_text())['packages']['node_modules/prettier']['version']=='3.5.3'
    output=prefix/'node_modules/.bin/prettier';output.parent.mkdir(parents=True,exist_ok=True)
    output.write_text('#!/bin/bash\\necho 3.5.3\\n');output.chmod(0o755)
'''
    for name in ["curl", "tar", "go", "npm"]:
        executable(mock / name, mock_driver)
    executable(mock / "node", "#!/bin/bash\nexit 0\n")

    def run(case, system="Linux", arch="x86_64", **extra):
        installed = root / case
        (installed / "bin").mkdir(parents=True)
        for name in VERSIONS:
            executable(installed / "bin" / name, binary(name, "0.0.0"))
        log = root / f"{case}.log"
        log.touch()
        env = {**os.environ, "PATH": f"{mock}:/usr/bin:/bin",
               "QMC_TOOLS_DIR": str(installed), "QMC_TEST_OS": system,
               "QMC_TEST_ARCH": arch, "QMC_TEST_LOG": str(log),
               "QMC_TEST_ASSETS": str(assets), "QMC_TEST_TAR": shutil.which("tar"),
               **extra}
        result = subprocess.run(["bash", str(root / "scripts/setup-deps.sh")], env=env,
                                text=True, capture_output=True, timeout=30)
        return result, installed, log, env

    for index, (system, arch, tree_platform, shell_platform) in enumerate(platforms):
        result, installed, log, env = run(f"platform-{index}", system, arch)
        assert result.returncode == 0, result.stderr
        entries = [json.loads(line) for line in log.read_text().splitlines()]
        urls = [entry["args"][entry["args"].index("-o")-1] for entry in entries if entry["name"] == "curl"]
        assert urls == [
            f"https://github.com/numtide/treefmt/releases/download/v2.5.0/treefmt_2.5.0_{tree_platform}.tar.gz",
            f"https://github.com/koalaman/shellcheck/releases/download/v0.11.0/shellcheck-v0.11.0.{shell_platform}.tar.gz",
        ], urls
        assert all(entry["sumdb"] == "sum.golang.org" and entry["nosumdb"] == "" for entry in entries if entry["name"] == "go")
        assert any(entry["name"] == "npm" and "--ignore-scripts" in entry["args"] for entry in entries)
        for tool in VERSIONS:
            command = f'source "{root}/tools/versions.sh"; qmc_tool_version {tool}'
            check = subprocess.run(["bash", "-c", command], env=env, capture_output=True, text=True, check=True)
            assert check.stdout.strip() == VERSIONS[tool], (tool, check.stdout)
        previous = log.read_text()
        again = subprocess.run(["bash", str(root / "scripts/setup-deps.sh")], env=env, capture_output=True, text=True, timeout=30)
        assert again.returncode == 0 and log.read_text() == previous, "matching installations were reinstalled"

    result, installed, log, _ = run("bad-checksum", QMC_TEST_CORRUPT="1")
    assert result.returncode != 0 and "checksum verification failed" in result.stderr
    assert not any(json.loads(line)["name"] == "tar" for line in log.read_text().splitlines()), "unverified archive was extracted"
    assert (installed / "bin/treefmt").read_bytes() == binary("treefmt", "0.0.0")
    result, _, _, _ = run("bad-compiled-version", QMC_TEST_WRONG_GO="1")
    assert result.returncode != 0 and "unexpected version" in result.stderr
    result, _, _, _ = run("npm-failure", QMC_TEST_NPM_FAIL="1")
    assert result.returncode == 42 and "Pinned tools are ready" not in result.stdout
    for system, arch in [("FreeBSD", "x86_64"), ("Linux", "riscv64")]:
        result, _, log, _ = run(f"unsupported-{system}-{arch}", system, arch)
        assert result.returncode != 0 and "supports" in result.stderr and not log.read_text()
    result, _, log, _ = run("relative-output", QMC_TOOLS_DIR="relative-tools")
    assert result.returncode != 0 and "absolute dedicated directory" in result.stderr and not log.read_text()

print("Tool setup passed: four platform selections, checksums before extraction, pinned versions, reuse, and failure propagation.")
