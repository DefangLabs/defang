import * as fs from 'fs';
import * as os from 'os';
import * as path from 'path';
import * as tar from 'tar';
import * as http from 'http';
import * as AdmZip from 'adm-zip';

type cbErrorHandler = (err?: Error | null) => void | null;
let platform = os.platform().toString();
let arch = os.arch().toString();

function downloadFile(version: string, filename: string, outputPath: string): string {
  const repo = 'defang-io/defang';
  const downloadUrl = `https://api.github.com/repos/${repo}/releases/tags/v${version}/${filename}`;

  const downloadTargetFile = path.join(outputPath, filename);
  const writeFileStream = fs.createWriteStream(downloadTargetFile);

  http.get(downloadUrl, (response) => {
    response.pipe(writeFileStream);
    writeFileStream.on('finish', () => {
      writeFileStream.close();
      console.log(`File downloaded to ${downloadTargetFile}`);
      return downloadTargetFile;
    });
  }).on('error', (err) => {
    fs.unlink(downloadTargetFile, () => {
      console.error(err);
    });
  });

  return "";
}

function extractArchive(archiveFilePath: string, outputPath: string, cb: cbErrorHandler): boolean {
  let result = false;
  const ext = path.extname(archiveFilePath).toLocaleLowerCase();
  switch (ext) {
    case '.zip':
      result = extractZip(archiveFilePath, outputPath, cb);
      break;
    case '.tar.gz':
      result = extractTarGz(archiveFilePath, outputPath, cb);
      break;
    default:
      cb(new Error(`Unsupported archive extension: ${ext}`));
  }

  return result;
}
function extractZip(zipPath: string, outputPath: string, cb: cbErrorHandler): boolean {
    let result = true;
    try {
        let zip = new AdmZip(zipPath);
        zip.extractAllTo(outputPath, true);
        cb?.();
    } catch (error) {
      result = false;
      cb(new Error(`An error occurred during ZIP extraction: ${error}`));
    }

    return result;
}

function extractTarGz(tarGzFilePath: string, outputPath: string, cb: cbErrorHandler): boolean  {
  let result = true;
  try {
    tar.extract({ cwd: outputPath, 
      file: tarGzFilePath, 
      sync: true,
      strict: true,
      });
  } catch (error) {
    result = false;
    cb?.(new Error(`An error occurred during TAR.GZ extraction: ${error}`));
  }

  return result;
}


function getVersion(filename: string): string {
  const data = fs.readFileSync(filename, 'utf8');
  const pkg = JSON.parse(data);
  return pkg.version;
}

function getAppFilename(version: string, os: string, cpu: string): string {
  let compression = 'tar.gz';
  switch (os) {
    case 'windows':
      platform = 'windows';
      compression = 'zip';
      break;
    case 'linux':
      platform = 'linux';
      break;
    case 'darwin':
      platform = 'macOS';
      break;
    default:
      throw new Error(`Unsupported operating system: ${os}`);
  }

  switch (cpu) {
    case 'x64':
      arch = 'amd64';
      break;
    case 'arm64':
      arch = 'arm64';
      break;
    default:
      throw new Error(`Unsupported architecture: ${arch}`);
  }

  if (platform === 'macOS') {
    return `defang_${version}_${os}.${compression}`;
  }
  return `defang_${version}_${os}_${arch}.${compression}`;
}

function install() {
  const version = getVersion('package.json');
  const filename = getAppFilename(version, os.platform(), os.arch());
  const destinationPath = path.join(__dirname, `../bin/${filename}`);
  const archiveFile = downloadFile(version, filename, destinationPath);
  if (archiveFile == "") {
    console.error(`Failed to download ${filename}!`);
    return;
  }
  
  const result = extractArchive(archiveFile, '../bin/', console.error)
  if (result === false) {
    console.error(`Failed to install binaries!`);
  } else {
    console.log(`Successfully installed binaries!`);
  }
}

install();