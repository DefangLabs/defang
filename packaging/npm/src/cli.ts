#!/usr/bin/env node

import * as child_process from 'child_process';

const EXECUTABLE = 'defang';

function getPathToExecutable(): string {
  let arch = process.arch.toString();
  let os = process.platform.toString(); 

  let extension = '';
  if (['win32', 'cygwin'].includes(process.platform)) {
    os = 'windows';
    extension = '.exe';
  }

  const executablePath = `${EXECUTABLE}${extension}`;
  try {
    return require.resolve(executablePath);
  } catch (e) {
    throw new Error(`Could not find application binary at ${executablePath}.`);
  }
}

function run(): void {
  const args = process.argv.slice(2);
  const processResult = child_process.spawnSync(getPathToExecutable(), args, { stdio: 'inherit' });
  process.exit(processResult.status ?? 0);
}

run();