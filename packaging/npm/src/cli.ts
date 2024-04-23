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

function run(): void {
  const args = process.argv.slice(2);
  const processResult = child_process.spawnSync(getPathToExecutable(), args, {
    stdio: "inherit",
  });
  process.exit(processResult.status ?? 0);
}

run();
