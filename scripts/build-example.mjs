import { fileURLToPath } from 'node:url';
import { spawnSync } from 'node:child_process';

const project = fileURLToPath(new URL('../examples/installer/', import.meta.url));
const args = ['build'];
if (process.platform === 'linux') args.push('-tags', 'webkit2_41');
const env = process.platform === 'darwin'
  ? { ...process.env, CGO_LDFLAGS: `${process.env.CGO_LDFLAGS ?? ''} -framework UniformTypeIdentifiers`.trim() }
  : process.env;
const result = spawnSync('wails', args, { cwd: project, stdio: 'inherit', shell: false, env });
if (result.error) throw result.error;
process.exitCode = result.status ?? 1;
