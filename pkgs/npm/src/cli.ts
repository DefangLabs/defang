#!/usr/bin/env node

import * as path from "path";
import * as child_process from "child_process";

const EXECUTABLE = "defang";

function getPathToExecutable(): string {
  let extension = "";
  if (["win32", "cygwin"].includes(process.platform)) {
    extension = ".exe";
  }

  const executablePath = path.join(__dirname, `${EXECUTABLE}${extension}`);
  try {
    return require.resolve(executablePath);
  } catch (e) {
    throw new Error(`Could not find application binary at ${executablePath}.`);
  }
}

// js wrapper to use by npx or npm exec, this will call the defang binary with
// the arguments passed to the npx line. NPM installer will create a symlink
// in the user PATH to the cli.js to execute. The symlink will name the same as
// the package name i.e. defang.
function run(): void {
  let processResult: child_process.SpawnSyncReturns<Buffer> | null = null;
  try {
    const args = process.argv.slice(2);
    processResult = child_process.spawnSync(getPathToExecutable(), args, {
      stdio: "inherit",
    });
  } catch (error) {
    console.error(error);
    process.exitCode = processResult?.status ?? 1;
  }
}

run();
