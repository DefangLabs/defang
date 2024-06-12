#!/usr/bin/env node

import * as path from "path";
import * as child_process from "child_process";
import { promisify } from "util";
import { downloadAppArchive } from "./install";

const exec = promisify(child_process.exec);
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

function extractCLIVersions(output: string): {
  defangCLI: string;
  latestCLI: string;
} {
  const defangCLIRegex = /Defang CLI:\s+(\w+)/;
  const latestCLIRegex = /Latest CLI:\s+(\S+)/;

  const defangCLIMatch = output.match(defangCLIRegex);
  const latestCLIMatch = output.match(latestCLIRegex);

  if (defangCLIMatch && latestCLIMatch) {
    return {
      defangCLI: defangCLIMatch[1],
      latestCLI: latestCLIMatch[1],
    };
  } else {
    throw new Error("Could not extract CLI versions from the output.");
  }
}

async function getVersionInfo(): Promise<{ current: string; latest: string }> {
  const execPath = getPathToExecutable();

  // Exec output contains both stderr and stdout outputs
  const versionInfo = await exec(execPath + " --version");

  const result = extractCLIVersions(versionInfo.stdout);

  return { current: result.defangCLI, latest: result.latestCLI };
}

// js wrapper to use by npx or npm exec, this will call the defang binary with
// the arguments passed to the npx line. NPM installer will create a symlink
// in the user PATH to the cli.js to execute. The symlink will name the same as
// the package name i.e. defang.
async function run(): Promise<void> {
  try {
    const { current, latest } = await getVersionInfo();

    if (current != latest) {
      // download and install the latest version of defang cli
      downloadAppArchive();
    }

    const args = process.argv.slice(2);
    const processResult = child_process.spawnSync(getPathToExecutable(), args, {
      stdio: "inherit",
    });

    processResult.error && console.error(processResult.error);
    process.exitCode = processResult.status ?? 1;
  } catch (error) {
    console.error(error);
    process.exitCode = 2;
  }
}

run();
