// Import the functions you want to test from cli.ts
import "mocha";
import sinon from "sinon";
import * as chai from "chai";
import chaiAsPromised from "chai-as-promised";

import * as childProcs from "child_process";
import clilib from "../src/clilib.ts";
import { run, install } from "../src/cliexec.ts";

chai.use(chaiAsPromised);
const { assert, expect } = chai;

var sandbox: sinon.SinonSandbox;
describe("Testing install", () => {
  let downloadAppArchiveStub: sinon.SinonStub;
  let extractArchiveStub: sinon.SinonStub;
  let deleteArchiveStub: sinon.SinonStub;

  beforeEach(() => {
    sandbox = sinon.createSandbox();
    downloadAppArchiveStub = sandbox
      .stub(clilib, "downloadAppArchive")
      .resolves("downloadFile.tar.gz");
    extractArchiveStub = sandbox.stub(clilib, "extractArchive").resolves(true);
    deleteArchiveStub = sandbox
      .stub(clilib, "deleteArchive")
      .callsFake(() => Promise.resolve());
  });

  afterEach(() => {
    sandbox.restore();
  });

  it("sanity", async () => {
    const os = { platform: "windows", arch: "x64" };
    await install("0.5.32", "somedir", os);
    sinon.assert.calledOnce(downloadAppArchiveStub);
    sinon.assert.calledOnce(extractArchiveStub);
    sinon.assert.calledOnce(deleteArchiveStub);
  });

  it("archive file name failed", async () => {
    const os = { platform: "OS32", arch: "x64" };
    try {
      await install("0.5.32", "somedir", os);
      expect.fail("Expected an error to be thrown");
    } catch (error) {
      sinon.assert.notCalled(downloadAppArchiveStub);
      sinon.assert.notCalled(extractArchiveStub);
      sinon.assert.notCalled(deleteArchiveStub);
    }
  });

  it("download failed", async () => {
    downloadAppArchiveStub.throws(new Error("Download failed"));
    const os = { platform: "win32", arch: "x64" };
    try {
      await install("0.5.32", "somedir", os);
      expect.fail("Expected an error to be thrown");
    } catch (error) {
      sinon.assert.calledOnce(downloadAppArchiveStub);
      sinon.assert.notCalled(extractArchiveStub);
      sinon.assert.notCalled(deleteArchiveStub);
    }
  });

  it("extract failed", async () => {
    extractArchiveStub.returns(false);
    const os = { platform: "win32", arch: "x64" };
    try {
      await install("0.5.32", "somedir", os);
      expect.fail("Expected an error to be thrown");
    } catch (error) {
      sinon.assert.calledOnce(downloadAppArchiveStub);
      sinon.assert.calledOnce(extractArchiveStub);
      sinon.assert.notCalled(deleteArchiveStub);
    }
  });
});

describe("Testing install", () => {
  let extractCLIWrapperArgsStub: sinon.SinonStub;
  let getVersionInfoStub: sinon.SinonStub;
  let getPathToExecutableStub: sinon.SinonStub;
  let childProcStub: sinon.SinonStub;

  beforeEach(() => {
    sandbox = sinon.createSandbox();
    extractCLIWrapperArgsStub = sandbox.stub(clilib, "extractCLIWrapperArgs");
    getVersionInfoStub = sandbox.stub(clilib, "getVersionInfo");
    getPathToExecutableStub = sandbox.stub(clilib, "getPathToExecutable");
    //childProcStub = sandbox.stub(childProcs, "spawnSync");

    extractCLIWrapperArgsStub.returns({
      cliParams: { uselatest: false },
      outArgs: [],
    });
    getVersionInfoStub.resolves({ current: "0.5.24", latest: "0.5.32" });
    getPathToExecutableStub.returns("path/to/defang/cli.js");
    //childProcStub.returns({ status: 0 });
  });

  afterEach(() => {
    sandbox.restore();
  });

  it("sanity - not installing latest", async () => {
    run();
    sinon.assert.calledOnce(extractCLIWrapperArgsStub);
    sinon.assert.notCalled(getVersionInfoStub);
    sinon.assert.calledOnce(getPathToExecutableStub);
  });

  it("sanity - not installing latest", async () => {
    getPathToExecutableStub.returns(null);
    try {
      run();
      expect.fail("Expected an error to be thrown");
    } catch (error) {
      sinon.assert.calledOnce(extractCLIWrapperArgsStub);
      sinon.assert.notCalled(getVersionInfoStub);
      sinon.assert.calledOnce(getPathToExecutableStub);
    }
  });
});
