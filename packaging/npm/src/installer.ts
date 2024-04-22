import * as fs from "graceful-fs";
import * as os from "os";
import * as path from "path";
import * as tar from "tar";
import * as AdmZip from "adm-zip";
import axios from "axios";

async function downloadAppArchive(
  version: string,
  archiveFilename: string,
  outputPath: string
): Promise<string> {
  const repo = "defang-io/defang";
  const downloadUrl = `https://github.com/${repo}/releases/download/v${version}/${archiveFilename}`;
  const downloadTargetFile = path.join(outputPath, archiveFilename);

  return downloadFile(downloadUrl, downloadTargetFile);
}

async function downloadFile(
  downloadUrl: string,
  downloadTargetFile: string
): Promise<string> {
  try {
    const writeFileStream = fs.createWriteStream(downloadTargetFile);

    console.log(`Downloading ${downloadUrl}`);
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

    fs.writeFileSync(downloadTargetFile, response.data);
    return downloadTargetFile;
  } catch (error) {
    console.error(`Failed to download ${downloadUrl}. ${error}`);
    fs.unlinkSync(downloadTargetFile);
    return "";
  }
}

function extractArchive(archiveFilePath: string, outputPath: string): boolean {
  let result = false;

  console.log(`Extracting ${archiveFilePath}`);
  const ext = path.extname(archiveFilePath).toLocaleLowerCase();
  switch (ext) {
    case ".zip":
      result = extractZip(archiveFilePath, outputPath);
      break;
    case ".gz":
      result = extractTarGz(archiveFilePath, outputPath);
      break;
    default:
      throw new Error(`Unsupported archive extension: ${ext}`);
  }

  return result;
}

function extractZip(zipPath: string, outputPath: string): boolean {
  try {
    const zip = new AdmZip(zipPath);
    zip.extractAllTo(outputPath, true, true);
    return true;
  } catch (error) {
    console.error(`An error occurred during zip extraction: ${error}`);
    return false;
  }
}

function extractTarGz(tarGzFilePath: string, outputPath: string): boolean {
  try {
    tar.extract({
      cwd: outputPath,
      file: tarGzFilePath,
      sync: true,
      strict: true,
    });
    return true;
  } catch (error) {
    console.error(`An error occurred during tar.gz extraction: ${error}`);
    return false;
  }
}

function deleteArchive(archiveFilePath: string): void {
  fs.unlinkSync(archiveFilePath);
}

function getVersion(filename: string): string {
  const data = fs.readFileSync(filename, "utf8");
  const pkg = JSON.parse(data);
  return pkg.version;
}

function getAppArchiveFilename(
  version: string,
  platform: string,
  arch: string
): string {
  let compression = "zip";
  switch (platform) {
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

async function install() {
  try {
    console.log(`Starting install of defang cli`);

    const version = getVersion("package.json");
    const filename = getAppArchiveFilename(version, os.platform(), os.arch());
    const archiveFile = await downloadAppArchive(version, filename, __dirname);
    if (archiveFile.length === 0) {
      throw new Error(`Failed to download ${filename}`);
    }

    const result = extractArchive(archiveFile, "./bin");
    if (result === false) {
      throw new Error(`Failed to install binaries!`);
    }
    console.log(`Successfully installed defang cli!`);
    deleteArchive(archiveFile);
  } catch (error) {
    console.error(error);
    process.exit(1);
  }
}

install();
