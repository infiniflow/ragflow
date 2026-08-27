import { execSync } from 'node:child_process';
import { dirname, join } from 'node:path';
import process from 'node:process';
import { fileURLToPath } from 'node:url';

const __filename = fileURLToPath(import.meta.url);
const __dirname = dirname(__filename);
const repoRoot = dirname(__dirname);
const isWindows = process.platform === 'win32';

// Use the locally installed lefthook binary to avoid relying on a global install or PATH.
const lefthookBin = join(
  repoRoot,
  'node_modules',
  '.bin',
  `lefthook${isWindows ? '.cmd' : ''}`,
);

try {
  // Verify we are inside a Git repository first.
  execSync('git rev-parse --git-dir', {
    cwd: repoRoot,
    stdio: 'ignore',
  });

  // Install lefthook hooks from the repository root.
  execSync(`"${lefthookBin}" install`, {
    cwd: repoRoot,
    stdio: 'inherit',
  });
} catch {
  // Silently ignore failures (not a Git repo or lefthook install failed) so npm install is not interrupted.
}
