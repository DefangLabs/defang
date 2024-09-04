import AdmZip from "adm-zip";
import axios from "axios";
import * as child_process from "child_process";
import * as fs from "fs";
import * as path from "path";
import * as tar from "tar";
import { promisify } from "util";

// regex to match semantic version (from semver.org)
const SEMVER_REGEX =
  /^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-((?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*)(?:\.(?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*))*))?(?:\+([0-9a-zA-Z-]+(?:\.[0-9a-zA-Z-]+)*))?$/;

const EXECUTABLE = "defang";
const URL_LATEST_RELEASE =
  "https://api.github.com/repos/DefangLabs/defang/releases/latest";
const HTTP_STATUS_OK = 200;

const exec = promisify(child_process.exec);
async function getLatestVersion(): Promise<string> {
  const response = await axios.get(URL_LATEST_RELEASE);
  if (response?.status !== HTTP_STATUS_OK) {
    throw new Error(
      `Failed to get latest version from GitHub. Status code: ${response.status}`
    );
  }

  return response.data?.tag_name?.replace("v", "").trim();
}

async function downloadAppArchive(
  archiveFilename: string,
  outputPath: string
): Promise<string | null> {
  const downloadUrl = `https://s.defang.io/${archiveFilename}?x-defang-source=npm`;
  const downloadTargetFile = path.join(outputPath, archiveFilename);

  return await downloadFile(downloadUrl, downloadTargetFile);
}

async function downloadFile(
  downloadUrl: string,
  downloadTargetFile: string
): Promise<string | null> {
  try {
    const response = await axios.get(downloadUrl, {
      responseType: "arraybuffer",
      headers: {
        "Content-Type": "application/octet-stream",
      },
    });

    if (response?.data === undefined) {
      throw new Error(
        `Failed to download ${downloadUrl}. No data in response.`
      );
    }

    // write data to file, will overwrite if file already exists
    await fs.promises.writeFile(downloadTargetFile, response.data);

    return downloadTargetFile;
  } catch (error) {
    console.error(error);

    // something went wrong, clean up by deleting the downloaded file if it exists
    await fs.promises.unlink(downloadTargetFile);
    return null;
  }
}

async function extractArchive(
  archiveFilePath: string,
  outputPath: string
): Promise<boolean> {
  let result = false;

  const ext = path.extname(archiveFilePath).toLocaleLowerCase();
  switch (ext) {
    case ".zip":
      result = await extractZip(archiveFilePath, outputPath);
      break;
    case ".gz":
      result = extractTarGz(archiveFilePath, outputPath);
      break;
    default:
      return false; // unsupported archive extension
  }

  return result;
}

async function extractZip(
  zipPath: string,
  outputPath: string
): Promise<boolean> {
  try {
    const zip = new AdmZip(zipPath);
    const result = zip.extractEntryTo(EXECUTABLE, outputPath, true, true);
    await fs.promises.chmod(path.join(outputPath, EXECUTABLE), 755);
    return result;
  } catch (error) {
    console.error(`An error occurred during zip extraction: ${error}`);
    return false;
  }
}

function extractTarGz(tarGzFilePath: string, outputPath: string): boolean {
  try {
    tar.extract(
      {
        cwd: outputPath,
        file: tarGzFilePath,
        sync: true,
        strict: true,
      },
      [EXECUTABLE]
    );
    return true;
  } catch (error) {
    console.error(`An error occurred during tar.gz extraction: ${error}`);
    return false;
  }
}

async function deleteArchive(archiveFilePath: string): Promise<void> {
  await fs.promises.unlink(archiveFilePath);
}

function getAppArchiveFilename(
  version: string,
  platform: string,
  arch: string
): string {
  let compression = "zip";

  if (!SEMVER_REGEX.test(version)) {
    throw new Error(`Unsupported version: ${version}`);
  }

  switch (platform) {
    case "win32":
    case "windows":
      platform = "windows";
      break;
    case "linux":
      platform = "linux";
      compression = "tar.gz";
      break;
    case "darwin":
      platform = "macOS";
      break;
    default:
      throw new Error(`Unsupported operating system: ${platform}`);
  }

  switch (arch) {
    case "x64":
      arch = "amd64";
      break;
    case "arm64":
      arch = "arm64";
      break;
    default:
      throw new Error(`Unsupported architecture: ${arch}`);
  }

  if (platform === "macOS") {
    return `defang_${version}_${platform}.${compression}`;
  }
  return `defang_${version}_${platform}_${arch}.${compression}`;
}

function getPathToExecutable(): string | null {
  let extension = "";
  if (["win32", "cygwin"].includes(process.platform)) {
    extension = ".exe";
  }

  const executablePath = path.join(__dirname, `${EXECUTABLE}${extension}`);
  try {
    return require.resolve(executablePath);
  } catch (e) {
    return null;
  }
}

function extractCLIVersions(versionInfo: string): {
  defangCLI: string;
  latestCLI: string;
} {
  // parse the CLI version info
  // e.g.
  // Defang CLI:    v0.5.24
  // Latest CLI:    v0.5.24
  // Defang Fabric: v0.5.0-643-abcdef012
  //

  const versionRegex = /\d+\.\d+\.\d+/g;
  const matches = versionInfo.match(versionRegex);

  if (matches != null && matches.length >= 2) {
    return {
      defangCLI: matches[0],
      latestCLI: matches[1],
    };
  } else {
    throw new Error("Could not extract CLI versions from the output.");
  }
}

export type VersionInfo = {
  current: string | null;
  latest: string | null;
};

async function getVersionInfo(): Promise<VersionInfo> {
  let result: VersionInfo = { current: null, latest: null };
  try {
    const execPath = getPathToExecutable();

    if (!execPath) {
      // there is no executable, so we can't get the version info from the CLI
      const latestVersion = await getLatestVersion();

      return { current: null, latest: latestVersion };
    }

    // Exec output contains both stderr and stdout outputs
    const versionInfo = await exec(execPath + " version");

    const verInfo = extractCLIVersions(versionInfo.stdout);
    result.current = verInfo.defangCLI;
    result.latest = verInfo.latestCLI;
  } catch (error) {
    console.error(error);
  }

  return result;
}

type CLIParams = {
  uselatest: boolean;
};

function extractCLIWrapperArgs(args: string[]): {
  cliParams: CLIParams;
  outArgs: string[];
} {
  // set defaults
  const cliParams: CLIParams = {
    uselatest: true, //default to use the latest version of defang cli
  };

  const outArgs: string[] = [];

  // extract out the CLI wrapper parameters
  for (const arg of args) {
    const argLower = arg.toLowerCase().replaceAll(" ", "");
    if (argLower.startsWith("--use-latest")) {
      const startOfValue = argLower.indexOf("=");
      if (startOfValue >= 0) {
        if (argLower.slice(startOfValue + 1) == "false") {
          cliParams.uselatest = false;
        }
      }
    } else {
      outArgs.push(arg);
    }
  }

  return { cliParams, outArgs };
}

function getEndNameFromPath(pathLine: string): string {
  const executableName = path.basename(pathLine);

  return executableName.split(".")[0];
}

export async function install(
  version: string,
  saveDirectory: string,
  os: { platform: string; arch: string }
) {
  try {
    console.log(`Getting latest defang cli`);

    // download the latest version of defang cli
    const filename = getAppArchiveFilename(version, os.platform, os.arch);

    const archiveFile = await downloadAppArchive(
      filename,
      saveDirectory
    );

    if (archiveFile == null || archiveFile.length === 0) {
      throw new Error(`Failed to download ${filename}`);
    }

    // Because the releases are compressed tar.gz or .zip we need to
    // uncompress them to the ./bin directory in the package in node_modules.
    const result = await extractArchive(archiveFile, saveDirectory);
    if (result === false) {
      throw new Error(`Failed to install binaries!`);
    }

    // Delete the downloaded archive since we have successfully downloaded
    // and uncompressed it.
    await deleteArchive(archiveFile);
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
    const { cliParams, outArgs: args } = extractCLIWrapperArgs(
      process.argv.slice(2)
    );

    if (cliParams.uselatest) {
      const { current, latest }: VersionInfo = await getVersionInfo();

      // get the latest version of defang cli if not already installed
      if (latest != null && (current == null || current != latest)) {
        await install(latest, __dirname, {
          platform: process.platform,
          arch: process.arch,
        });
      }
    }

    // execute the defang binary with the arguments passed to the npx line.
    const pathToExec = getPathToExecutable();
    if (!pathToExec) {
      throw new Error("Could not find the defang executable.");
    }

    const commandline = ["npx", getEndNameFromPath(pathToExec)]
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

const clilib = {
  deleteArchive,
  downloadAppArchive,
  downloadFile,
  extractArchive,
  extractCLIVersions,
  extractCLIWrapperArgs,
  getAppArchiveFilename,
  getEndNameFromPath,
  getLatestVersion,
  getVersionInfo,
  getPathToExecutable,
};

export default clilib;
