import {
  expect,
  jest,
  it,
  describe,
  beforeEach,
  afterEach,
} from "@jest/globals";
import axios, { AxiosResponse } from "axios";
import * as fs from "fs/promises";
jest.mock("axios");
jest.mock("fs/promises");

// Import the functions you want to test from cli.ts
import {
  // ... import the functions you want to test
  getLatestVersion,
  downloadFile,
  getAppArchiveFilename,
  extractCLIVersions,
  // getVersionInfo,
  // extractCLIWrapperArgs,
  // getEndNameFromPath,
  // install,
  // run,
} from "./clilib";

const mockedAxios = axios as jest.Mocked<typeof axios>;

describe("Testing getLatestVersion()", () => {
  afterEach(() => {
    jest.resetAllMocks();
  });

  it("happy path", async () => {
    const mockResponse: AxiosResponse = {
      status: 200,
      data: {
        tag_name: "v0.5.32",
      },
    } as AxiosResponse;
    mockedAxios.get.mockResolvedValue(mockResponse);

    const latestVersion = await getLatestVersion();
    expect(latestVersion).toBe("0.5.32");
  });

  it("bad HTTP Status", async () => {
    const mockResponse: AxiosResponse = {
      status: 500,
    } as AxiosResponse;
    mockedAxios.get.mockResolvedValue(mockResponse);

    const t = async () => {
      await getLatestVersion();
    };
    expect(t).rejects.toThrow();
  });

  it("empty tag_name", async () => {
    const mockResponse: AxiosResponse = {
      status: 200,
      data: {
        tag_name: "",
      },
    } as AxiosResponse;
    mockedAxios.get.mockResolvedValue(mockResponse);

    const latestVersion = await getLatestVersion();
    expect(latestVersion).toBe("");
  });

  it("ill-formed tag_name", async () => {
    const mockResponse: AxiosResponse = {
      status: 200,
      data: {},
    } as AxiosResponse;
    mockedAxios.get.mockResolvedValue(mockResponse);

    const latestVersion = await getLatestVersion();
    expect(latestVersion).toBeUndefined();
  });
});

describe("Testing downloadFile()", () => {
  let downloadFileName = "target";
  const url = "url";
  const header = {
    responseType: "arraybuffer",
    headers: {
      "Content-Type": "application/octet-stream",
    },
  };

  beforeEach(() => {
    downloadFileName = "target";
    const mockResponse: AxiosResponse = {
      status: 200,
      data: {},
    } as AxiosResponse;

    mockedAxios.get.mockResolvedValue(mockResponse);
    const writeFileMock = fs.writeFile as jest.MockedFunction<
      typeof fs.writeFile
    >;
    const unlinkMock = fs.unlink as jest.MockedFunction<typeof fs.unlink>;

    writeFileMock.mockResolvedValue();
    unlinkMock.mockResolvedValue();
  });

  afterEach(() => {
    jest.resetAllMocks();
  });

  it("happy path", async () => {
    const targetFile = await downloadFile(url, downloadFileName);

    expect(targetFile).toBe(downloadFileName);
    expect(mockedAxios.get).toBeCalledWith(
      "url",
      expect.objectContaining({ responseType: "arraybuffer" })
    );
    expect(fs.writeFile).toBeCalledWith(targetFile, {});
    expect(fs.unlink).not.toHaveBeenCalled();
  });

  it("download fails path", async () => {
    mockedAxios.get.mockRejectedValue("failed");
    const targetFile = await downloadFile(url, downloadFileName);

    expect(targetFile).toBeNull();
    expect(mockedAxios.get).toBeCalledWith(
      url,
      expect.objectContaining(header)
    );
    expect(fs.writeFile).not.toHaveBeenCalled();
    expect(fs.unlink).toBeCalledWith(downloadFileName);
  });

  it("write failed", async () => {
    const writeFileMock = fs.writeFile as jest.MockedFunction<
      typeof fs.writeFile
    >;

    writeFileMock.mockRejectedValue(new Error("failed"));
    const targetFile = await downloadFile(url, downloadFileName);

    expect(targetFile).toBeNull();
    expect(mockedAxios.get).toBeCalledWith(
      url,
      expect.objectContaining(header)
    );
    expect(fs.unlink).toHaveBeenCalled();
  });
});

describe("Testing getAppArchiveFilename()", () => {
  it.each([
    ["0.1.0", "windows", "x64", "defang_0.1.0_windows_amd64.zip"],
    ["0.2.9", "windows", "arm64", "defang_0.2.9_windows_arm64.zip"],
    ["0.3.10", "linux", "x64", "defang_0.3.10_linux_amd64.tar.gz"],
    ["0.4.21", "linux", "arm64", "defang_0.4.21_linux_arm64.tar.gz"],
    ["0.5.45", "darwin", "arm64", "defang_0.5.45_macOS.zip"],
    ["0.5.45", "darwin", "x64", "defang_0.5.45_macOS.zip"],
  ])("happy path", (version, platform, arch, expected) =>
    expect(getAppArchiveFilename(version, platform, arch)).toBe(expected)
  );

  it.failing.each([
    ["", "windows", "x64"],
    ["0.2.9", "windows", "risc64"],
    ["0.5.45", "darwin", "powerpc"],
  ])("unknown types", (version, platform, arch) =>
    expect(getAppArchiveFilename(version, platform, arch)).toThrowError()
  );
});

describe("Testing extractCLIVersions()", () => {
  it("happy path", async () => {
    const versionInfo =
      "Defang CLI: v0.5.24\nLatest CLI: v0.5.32\nDefang Fabric: v0.5.0-643";
    const expected = { defangCLI: "0.5.24", latestCLI: "0.5.32" };

    expect(extractCLIVersions(versionInfo)).toStrictEqual(expected);
  });

  it("missing v in version text", async () => {
    const versionInfo =
      "Defang CLI: 0.5.24\nLatest CLI: 0.5.32\nDefang Fabric: v0.5.0-643";
    const expected = { defangCLI: "0.5.24", latestCLI: "0.5.32" };

    expect(extractCLIVersions(versionInfo)).toStrictEqual(expected);
  });

  it.failing("missing Defang CLI", () => {
    const versionInfo =
      "Defang CLI: \nLatest CLI: v0.5.32\nDefang Fabric: v0.5.0-643";
    expect(extractCLIVersions(versionInfo)).toThrowError();
  });

  it.failing("missing Latest CLI", () => {
    const versionInfo =
      "Defang CLI: v0.5.24\nLatest CLI: \nDefang Fabric: v0.5.0-643";
    expect(extractCLIVersions(versionInfo)).toThrowError();
  });

  it("no fabric version in input", async () => {
    const versionInfo = "Defang CLI: v0.5.24\nLatest CLI: v0.5.32\n";
    const expected = { defangCLI: "0.5.24", latestCLI: "0.5.32" };

    expect(extractCLIVersions(versionInfo)).toStrictEqual(expected);
  });
});

describe("Testing install", () => {});
