// Verify actual required formatters/checkers fail, using isolated Git worktrees.
import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import {
  chmod,
  mkdir,
  mkdtemp,
  rm,
  symlink,
  writeFile,
} from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const temporary = await mkdtemp(path.join(os.tmpdir(), "qmc-format-test-"));
const format = path.join(root, "scripts/format.sh");
const tools = ["treefmt", "gofumpt", "gci", "shfmt", "prettier", "shellcheck"];
try {
  const workspace = path.join(temporary, "source");
  const install = path.join(temporary, "tools");
  await mkdir(workspace);
  await mkdir(path.join(install, "bin"), { recursive: true });
  const init = spawnSync("git", ["init", "--quiet", workspace]);
  assert.equal(init.status, 0, String(init.stderr));
  const realTools = new Map();
  for (const tool of tools) {
    const lookup = spawnSync(
      "bash",
      ["-c", 'command -v "$1"', "lookup", tool],
      { encoding: "utf8" },
    );
    assert.equal(
      lookup.status,
      0,
      `install ${tool} before testing format gates`,
    );
    realTools.set(tool, lookup.stdout.trim());
    await symlink(realTools.get(tool), path.join(install, "bin", tool));
  }
  const environment = { ...process.env, QMC_TOOLS_DIR: install };
  const run = (env = environment) =>
    spawnSync("bash", [format, "check", workspace], {
      env,
      encoding: "utf8",
      timeout: 30000,
    });
  const expectSuccess = (result) =>
    assert.equal(result.status, 0, result.stdout + result.stderr);
  const good = {
    "fixture.go": "package fixture\n\nfunc Example() {}\n",
    "fixture.md": "# Heading\n\n- item\n",
    "fixture.json": '{\n  "name": "fixture"\n}\n',
    "fixture.yml": "name: fixture\n",
    "fixture.js": "const value = 1;\n",
    "fixture.mjs": "export const value = 1;\n",
    "fixture.css": "p {\n  color: red;\n}\n",
    "fixture.html": "<p>Hello</p>\n",
    "fixture.sh": "#!/usr/bin/env bash\nset -euo pipefail\nprintf 'ok\\n'\n",
  };
  for (const [name, content] of Object.entries(good))
    await writeFile(path.join(workspace, name), content);
  expectSuccess(run());
  const malformed = {
    "fixture.go": "package fixture\nfunc broken( {\n",
    "fixture.md": "# Heading    \n\n-    item\n",
    "fixture.json": '{"name":\n',
    "fixture.yml": "name: [unfinished\n",
    "fixture.js": "const = ;\n",
    "fixture.mjs": "export const = ;\n",
    "fixture.css": "p{color:red}\n",
    "fixture.html": "<p>Hello</p><p>there</p>\n",
    "fixture.sh": "#!/usr/bin/env bash\necho $undefined_variable\n",
  };
  for (const [name, content] of Object.entries(malformed)) {
    await writeFile(path.join(workspace, name), content);
    const failed = run();
    assert.notEqual(
      failed.status,
      0,
      `invalid ${name} passed required formatting`,
    );
    assert(
      failed.stdout.includes(name) || failed.stderr.includes(name),
      `failure did not identify ${name}: ${failed.stdout}${failed.stderr}`,
    );
    await writeFile(path.join(workspace, name), good[name]);
  }
  // ShellCheck diagnostics fail even when no formatter would change the file.
  const diagnostic =
    "#!/usr/bin/env bash\nprintf '%s\\n' \"$undefined_variable\"\n";
  await writeFile(path.join(workspace, "fixture.sh"), diagnostic);
  const shellFailure = run();
  assert.notEqual(shellFailure.status, 0);
  assert.match(shellFailure.stderr + shellFailure.stdout, /SC2154/);
  await writeFile(path.join(workspace, "fixture.sh"), good["fixture.sh"]);
  for (const tool of tools) {
    const entry = path.join(install, "bin", tool);
    // Unlink owned symlinks before writing; never modify their real targets.
    await rm(entry);
    await writeFile(entry, "#!/qmc-unavailable-interpreter\n");
    await chmod(entry, 0o755);
    const failed = run();
    assert.notEqual(failed.status, 0, `unavailable ${tool} passed`);
    assert.match(failed.stderr, new RegExp(`${tool} .* is required`));
    await rm(entry);
    await symlink(realTools.get(tool), entry);
  }
  const prettier = path.join(install, "bin/prettier");
  await rm(prettier);
  await writeFile(prettier, "#!/bin/bash\necho 3.5.30\n");
  await chmod(prettier, 0o755);
  assert.notEqual(run().status, 0, "wrong-version prefix was accepted");
  await rm(prettier);
  await symlink(realTools.get("prettier"), prettier);
  // Environment overrides must not skip a broken required language.
  await writeFile(path.join(workspace, "fixture.js"), malformed["fixture.js"]);
  assert.notEqual(
    run({
      ...environment,
      TREEFMT_ALLOW_MISSING_FORMATTER: "true",
      TREEFMT_FORMATTERS: "gofumpt",
      TREEFMT_EXCLUDES: "*",
    }).status,
    0,
    "treefmt overrides weakened required checks",
  );
  await writeFile(path.join(workspace, "fixture.js"), good["fixture.js"]);
  expectSuccess(run());
  console.log(
    "Format gates passed: nine file extensions, six unavailable tools, version mismatch, ShellCheck diagnostics, and hostile treefmt overrides.",
  );
} finally {
  await rm(temporary, { recursive: true, force: true });
}
