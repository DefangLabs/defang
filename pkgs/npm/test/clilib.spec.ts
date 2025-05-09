import axios, {type AxiosResponse } from "axios";
import * as chai from "chai";
import chaiAsPromised from "chai-as-promised";
import fs from "fs";
import "mocha";
import * as sinon from "sinon";
import clilib from "../src/clilib";

chai.use(chaiAsPromised);
const { assert, expect } = chai;

var sandbox: sinon.SinonSandbox;

describe("Testing getLatestVersion()", () => {
  beforeEach(() => {
    sandbox = sinon.createSandbox();
  });

  afterEach(() => {
    sandbox.restore();
  });

  it("sanity", async () => {
    const mockResponse: AxiosResponse = {
      status: 200,
      data: {
        tag_name: "v0.5.32",
      },
    } as AxiosResponse;
    sandbox.stub(axios, "get").returns(Promise.resolve(mockResponse));

    await expect(clilib.getLatestVersion()).to.eventually.equal("0.5.32");
  });

  it("bad HTTP Status", async () => {
    const mockResponse: AxiosResponse = {
      status: 500,
    } as AxiosResponse;
    sandbox.stub(axios, "get").returns(Promise.reject(mockResponse));

    await expect(clilib.getLatestVersion()).to.be.rejected;
  });

  it("empty tag_name", async () => {
    const mockResponse: AxiosResponse = {
      status: 200,
      data: {
        tag_name: "",
      },
    } as AxiosResponse;
    sandbox.stub(axios, "get").returns(Promise.resolve(mockResponse));

    await expect(clilib.getLatestVersion()).to.eventually.equal("");
  });

  it("ill-formed tag_name", async () => {
    const mockResponse: AxiosResponse = {
      status: 200,
      data: {},
    } as AxiosResponse;
    sandbox.stub(axios, "get").returns(Promise.resolve(mockResponse));

    await expect(clilib.getLatestVersion()).to.eventually.be.undefined;
  });
});

describe("Testing downloadFile()", () => {
  let axiosGetStub: sinon.SinonStub;
  let writeStub: sinon.SinonStub;
  let unlinkStub: sinon.SinonStub;

  let downloadFileName = "target";
  const url = "url";
  const header = {
    responseType: "arraybuffer",
    headers: {
      "Content-Type": "application/octet-stream",
    },
  };
  let mockResponse: AxiosResponse;

  beforeEach(() => {
    sandbox = sinon.createSandbox();

    downloadFileName = "target";
    mockResponse = {
      status: 200,
      data: {},
    } as AxiosResponse;

    axiosGetStub = sandbox
      .stub(axios, "get")
      .returns(Promise.resolve(mockResponse));
    writeStub = sandbox
      .stub(fs.promises, "writeFile")
      .callsFake(() => Promise.resolve());
    unlinkStub = sandbox
      .stub(fs.promises, "unlink")
      .callsFake(() => Promise.resolve());
  });

  afterEach(() => {
    sandbox.restore();
  });

  it("sanity", async () => {
    await expect(
      clilib.downloadFile(url, downloadFileName)
    ).to.eventually.equal(downloadFileName);

    sinon.assert.calledWith(axiosGetStub, url, header);
    sinon.assert.calledWith(writeStub, downloadFileName, mockResponse.data);
    sinon.assert.notCalled(unlinkStub);
  });

  it("download fails path", async () => {
    axiosGetStub.returns(Promise.reject("failed"));
    const targetFile = await clilib.downloadFile(url, downloadFileName);
    await expect(
      clilib.downloadFile(url, downloadFileName)
    ).to.eventually.equal(targetFile);

    sinon.assert.calledWith(axiosGetStub, url, header);
    sinon.assert.notCalled(writeStub);
    sinon.assert.calledWith(unlinkStub, downloadFileName);
  });

  it("write failed", async () => {
    writeStub.returns(Promise.reject("failed"));
    await expect(clilib.downloadFile(url, downloadFileName)).to.eventually.null;
    sinon.assert.calledWith(axiosGetStub, url, header);
    sinon.assert.calledWith(unlinkStub, downloadFileName);
  });
});

describe("Testing getAppArchiveFilename()", () => {
  it("returns expected filename", () => {
    const iterationData = [
      ["0.0.0", "win32", "x64", "defang_0.0.0_windows_amd64.zip"],
      ["0.1.0", "windows", "x64", "defang_0.1.0_windows_amd64.zip"],
      ["0.2.9", "windows", "arm64", "defang_0.2.9_windows_arm64.zip"],
      ["0.3.10", "linux", "x64", "defang_0.3.10_linux_amd64.tar.gz"],
      ["0.4.21", "linux", "arm64", "defang_0.4.21_linux_arm64.tar.gz"],
      ["0.5.45", "darwin", "arm64", "defang_0.5.45_macOS.zip"],
      ["0.5.45", "darwin", "x64", "defang_0.5.45_macOS.zip"],
    ] as const;
    const testFunc = (
      version: string,
      platform: string,
      arch: string,
      expected: string
    ) =>
      expect(clilib.getAppArchiveFilename(version, platform, arch)).to.be.equal(
        expected
      );
    iterationData.forEach((testData) => testFunc.call(null, ...testData));
  });

  it("check error cases", () => {
    const iterationData = [
      ["", "windows", "x64"],
      ["0.2.9", "windows", "risc64"],
      ["0.5.45", "darwin", "powerpc"],
    ] as const;
    const testFunc = (version: string, platform: string, arch: string) =>
      expect(() =>
        clilib.getAppArchiveFilename(version, platform, arch)
      ).to.throw();
    iterationData.forEach((testData) => testFunc.call(null, ...testData));
  });
});

describe("Testing extractCLIVersions()", () => {
  it("sanity", async () => {
    const versionInfo =
      "Defang CLI: v0.5.24\nLatest CLI: v0.5.32\nDefang Fabric: v0.7.0-643";
    const expected = { defangCLI: "0.5.24", latestCLI: "0.5.32" };

    expect(clilib.extractCLIVersions(versionInfo)).to.be.deep.equal(expected);
  });

  it("missing v in version text", async () => {
    const versionInfo =
      "Defang CLI: 0.5.24\nLatest CLI: 0.5.32\nDefang Fabric: v0.7.0-643";
    const expected = { defangCLI: "0.5.24", latestCLI: "0.5.32" };
    expect(clilib.extractCLIVersions(versionInfo)).to.be.deep.equal(expected);
  });

  it("missing Defang CLI", () => {
    const versionInfo =
      "Defang CLI: \nLatest CLI: v0.5.32\nDefang Fabric: v0.7.0-643";
    const expected = { defangCLI: null, latestCLI: "0.5.32" };
    expect(clilib.extractCLIVersions(versionInfo)).to.be.deep.equal(expected);
  });

  it("missing Latest CLI", () => {
    const versionInfo =
      "Defang CLI: v0.5.24\nLatest CLI: \nDefang Fabric: v0.7.0-643";
    const expected = { defangCLI: "0.5.24", latestCLI: null };
    expect(clilib.extractCLIVersions(versionInfo)).to.be.deep.equal(expected);
  });

  it("no fabric version in input", async () => {
    const versionInfo = "Defang CLI: v0.5.24\nLatest CLI: v0.5.32\n";
    const expected = { defangCLI: "0.5.24", latestCLI: "0.5.32" };
    expect(clilib.extractCLIVersions(versionInfo)).to.be.deep.equal(expected);
  });

  it("ill-formed semver v in version text", async () => {
    const versionInfo =
      "Defang CLI: a8f4c7a0\nLatest CLI: 0.5.32\nDefang Fabric: v0.5.0-643";
    const expected = { defangCLI: null, latestCLI: "0.5.32" };
    expect(clilib.extractCLIVersions(versionInfo)).to.be.deep.equal(expected);
  });

});
