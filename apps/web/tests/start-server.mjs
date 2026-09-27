import { mkdtempSync, cpSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { randomUUID } from 'node:crypto';
import { spawn, spawnSync } from 'node:child_process';

const image = process.env.TEST_IMAGE;
const directory = image ? undefined : mkdtempSync(join(tmpdir(), 'beanframe-web-test-'));
const container = `beanframe-browser-${randomUUID()}`;
if (directory) cpSync('testdata/ledger', directory, { recursive: true });

// CI tests the published release image. Local make test still runs the local
// build, with a private copy of the small fixture in either case.
const child = image
  ? spawn('docker', ['run', '--rm', '--name', container,
      '-p', '127.0.0.1:18762:8080', '-e', 'AUTH_DISABLED=true', '-e', 'LOG_LEVEL=warn', image],
      { stdio: 'inherit' })
  : spawn('./bin/beanframe', [], { stdio: 'inherit', env: {
      ...process.env, LEDGER_PATH: join(directory, 'main.beancount')
    } });

function cleanup() {
  if (image) spawnSync('docker', ['rm', '-f', container], { stdio: 'ignore', timeout: 15_000 });
  if (directory) rmSync(directory, { recursive: true, force: true });
}
for (const signal of ['SIGTERM', 'SIGINT']) process.on(signal, () => {
  if (image) spawnSync('docker', ['stop', '--time', '5', container], { stdio: 'ignore', timeout: 15_000 });
  else child.kill(signal);
});
child.on('error', error => { console.error(error); cleanup(); process.exitCode = 1; });
child.on('exit', code => { cleanup(); process.exitCode = code || 0; });
