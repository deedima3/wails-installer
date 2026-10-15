import { writeFileSync, rmSync } from 'node:fs';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { spawnSync } from 'node:child_process';

const root = fileURLToPath(new URL('../', import.meta.url));
const project = join(root, 'examples', 'installer');
function run(command, args, cwd) {
  const result = spawnSync(command, args, { cwd, stdio: 'inherit', shell: false });
  if (result.error) throw result.error;
  if (result.status !== 0) throw new Error(`${command} exited with ${result.status}`);
}

const args = ['generate', 'module'];
if (process.platform === 'linux') args.push('-tags', 'webkit2_41');
run('wails', args, project);
const check = join(project, 'frontend', 'src', 'generated-binding-check.ts');
let ownsCheck = false;
try {
  writeFileSync(check, `import * as bindings from '../wailsjs/go/installer/Service';
import { createClient } from '@wails-installer/svelte';
export const client = createClient(bindings);
`, { flag: 'wx' });
  ownsCheck = true;
  // Invoke the JS entrypoint directly. npm.cmd needs a shell on Windows.
  run(process.execPath, [join(root, 'node_modules', 'svelte-check', 'bin', 'svelte-check'), '--tsconfig', './tsconfig.json'], join(project, 'frontend'));
} finally {
  if (ownsCheck) rmSync(check, { force: true });
}
