import {
  expect,
  jest,
  it,
  describe,
  beforeEach,
  afterEach,
} from "@jest/globals";
import axios, { AxiosResponse } from "axios";
import { promises } from "fs";
jest.mock("axios");
jest.mock("fs/promises");

// Import the functions you want to test from cli.ts
import {
  // ... import the functions you want to test
  getLatestVersion,
  downloadFile,
  // downloadAppArchive,
  // getVersionInfo,
  // extractCLIWrapperArgs,
  // getEndNameFromPath,
  // install,
  // run,
} from "./cliLib";

const mockedAxios = axios as jest.Mocked<typeof axios>;
const mockedPromises = promises as jest.Mocked<typeof promises>;

describe("Testing getLatestVersion()", () => {
  afterEach(() => {
    jest.resetAllMocks();
  });

  it("green test", async () => {
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
  beforeEach(() => {});

  afterEach(() => {
    jest.resetAllMocks();
  });

  it("green test", async () => {
    const mockResponse: AxiosResponse = {
      status: 200,
      data: {},
    } as AxiosResponse;

    mockedAxios.get.mockResolvedValue(mockResponse);

    mockedPromises.writeFile.mockReturnValueOnce(Promise.resolve());
    mockedPromises.unlink.mockReturnValueOnce(Promise.resolve());

    const targetFile = await downloadFile("url", "target");

    expect(targetFile).toBe("target");
    expect(mockedAxios.get).toBeCalledWith("url");
    expect(mockedPromises.writeFile).toBeCalledWith(targetFile, {});
    expect(mockedPromises.unlink).not.toHaveBeenCalled();
  });
});
