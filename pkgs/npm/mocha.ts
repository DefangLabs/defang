import Mocha from 'mocha';
import path from 'path';
import fs from 'fs/promises';

async function main() {
  try {
    const mocha = new Mocha({
      // @ts-expect-error: MochaOptions doesn't type this but it's valid
      extension: ['ts']
    });

    const testDir = new URL('./test', import.meta.url);
    const entries = await fs.readdir(testDir.pathname);

    for (const file of entries) {
      if (file.endsWith('.spec.ts')) {
        const fullPath = path.resolve(testDir.pathname, file);
        mocha.addFile(fullPath);
      }
    }

    await mocha.loadFilesAsync();

    mocha.run(failures => {
      process.exitCode = failures ? 1 : 0;
    });
  } catch (err) {
    console.error("error in main():", err);
    process.exit(1);
  }
}

main();