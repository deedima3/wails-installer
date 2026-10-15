import { fileURLToPath } from 'node:url';
import { spawnSync } from 'node:child_process';

const project = fileURLToPath(new URL('../examples/installer/', import.meta.url));
const args = ['build'];
if (process.platform === 'linux') args.push('-tags', 'webkit2_41');
const result = spawnSync('wails', args, { cwd: project, stdio: 'inherit', shell: false });
if (result.error) throw result.error;
process.exitCode = result.status ?? 1;
