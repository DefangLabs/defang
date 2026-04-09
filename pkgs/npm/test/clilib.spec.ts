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
    sandbox.stub(global, "fetch").resolves({
      ok: true,
      json: () => Promise.resolve({ tag_name: "v0.5.32" }),
    } as unknown as Response);

    await expect(clilib.getLatestVersion()).to.eventually.equal("0.5.32");
  });

  it("bad HTTP Status", async () => {
    sandbox.stub(global, "fetch").resolves({
      ok: false,
      status: 500,
    } as Response);

    await expect(clilib.getLatestVersion()).to.be.rejected;
  });

  it("empty tag_name", async () => {
    sandbox.stub(global, "fetch").resolves({
      ok: true,
      json: () => Promise.resolve({ tag_name: "" }),
    } as unknown as Response);

    await expect(clilib.getLatestVersion()).to.eventually.be.undefined;
  });

  it("ill-formed tag_name", async () => {
    sandbox.stub(global, "fetch").resolves({
      ok: true,
      json: () => Promise.resolve({}),
    } as unknown as Response);

    await expect(clilib.getLatestVersion()).to.eventually.be.undefined;
  });

  it("non-semver tag_name", async () => {
    sandbox.stub(global, "fetch").resolves({
      ok: true,
      json: () => Promise.resolve({ tag_name: "nightly-20260408" }),
    } as unknown as Response);

    await expect(clilib.getLatestVersion()).to.eventually.be.undefined;
  });

  it("tag_name with v in version string", async () => {
    sandbox.stub(global, "fetch").resolves({
      ok: true,
      json: () => Promise.resolve({ tag_name: "v1.2.3-dev.4" }),
    } as unknown as Response);

    await expect(clilib.getLatestVersion()).to.eventually.equal("1.2.3-dev.4");
  });
});

describe("Testing downloadFile()", () => {
  let fetchStub: sinon.SinonStub;
  let writeStub: sinon.SinonStub;
  let unlinkStub: sinon.SinonStub;

  let downloadFileName = "target";
  const url = "url";

  beforeEach(() => {
    sandbox = sinon.createSandbox();

    downloadFileName = "target";

    fetchStub = sandbox.stub(global, "fetch").resolves({
      ok: true,
      arrayBuffer: () => Promise.resolve(new ArrayBuffer(0)),
    } as unknown as Response);
    writeStub = sandbox
      .stub(fs.promises, "writeFile")
      .callsFake(() => Promise.resolve());
    unlinkStub = sandbox
      .stub(fs.promises, "rm")
      .callsFake(() => Promise.resolve());
  });

  afterEach(() => {
    sandbox.restore();
  });

  it("sanity", async () => {
    await expect(
      clilib.downloadFile(url, downloadFileName)
    ).to.eventually.equal(downloadFileName);

    sinon.assert.calledWith(fetchStub, url);
    sinon.assert.calledOnce(writeStub);
    sinon.assert.notCalled(unlinkStub);
  });

  it("download fails path", async () => {
    fetchStub.returns(Promise.reject("failed"));
    const targetFile = await clilib.downloadFile(url, downloadFileName);
    await expect(
      clilib.downloadFile(url, downloadFileName)
    ).to.eventually.equal(targetFile);

    sinon.assert.calledWith(fetchStub, url);
    sinon.assert.notCalled(writeStub);
    sinon.assert.calledWith(unlinkStub, downloadFileName);
  });

  it("write failed", async () => {
    writeStub.returns(Promise.reject("failed"));
    await expect(clilib.downloadFile(url, downloadFileName)).to.eventually.null;
    sinon.assert.calledWith(fetchStub, url);
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
