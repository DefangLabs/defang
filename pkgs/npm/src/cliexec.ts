import * as child_process from "child_process";
import clilib, { type VersionInfo } from "./clilib";

export async function install(
  version: string,
  saveDirectory: string,
  os: { platform: string; arch: string }
) {
  try {
    console.log(`Getting latest defang cli`);

    // download the latest version of defang cli
    const filename = clilib.getAppArchiveFilename(
      version,
      os.platform,
      os.arch
    );

    const archiveFile = await clilib.downloadAppArchive(
      version,
      filename,
      saveDirectory
    );

    if (archiveFile == null || archiveFile.length === 0) {
      throw new Error(`Failed to download ${filename}`);
    }

    // Because the releases are compressed tar.gz or .zip we need to
    // uncompress them to the ./bin directory in the package in node_modules.
    const result = await clilib.extractArchive(archiveFile, saveDirectory);
    if (result === false) {
      throw new Error(`Failed to install binaries!`);
    }

    // Delete the downloaded archive since we have successfully downloaded
    // and uncompressed it.
    await clilib.deleteArchive(archiveFile);
  } catch (error) {
    console.error(error);
    throw error;
  }
}

// js wrapper to use by npx or npm exec, this will call the defang binary with
// the arguments passed to the npx line. NPM installer will create a symlink
// in the user PATH to the cli.js to execute. The symlink will name the same as
// the package name i.e. defang.
export async function run(): Promise<void> {
  try {
    const { cliParams, outArgs: args } = clilib.extractCLIWrapperArgs(
      process.argv.slice(2)
    );

    if (cliParams.uselatest) {
      const { current, latest }: VersionInfo = await clilib.getVersionInfo();

      // get the latest version of defang cli if not already installed
      if (latest != null && (current == null || current != latest)) {
        await install(latest, __dirname, {
          platform: process.platform,
          arch: process.arch,
        });
      }
    }

    // execute the defang binary with the arguments passed to the npx line.
    const pathToExec = clilib.getPathToExecutable();
    if (!pathToExec) {
      throw new Error("Could not find the defang executable.");
    }

    const commandline = ["npx", clilib.getEndNameFromPath(pathToExec)]
      .join(" ")
      .trim();

    const processResult = child_process.spawnSync(pathToExec, args, {
      stdio: "inherit",
      env: { ...process.env, DEFANG_COMMAND_EXECUTOR: commandline },
    });

    // if there was an error, print it to the console.
    processResult.error && console.error(processResult.error);
    process.exitCode = processResult.status ?? 1;
  } catch (error) {
    console.error(error);
    process.exitCode = 2;
  }
}
