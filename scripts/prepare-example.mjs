import { mkdirSync, writeFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { spawnSync } from 'node:child_process';

const root = fileURLToPath(new URL('../', import.meta.url));
const project = join(root, 'examples', 'installer');
const executable = process.platform === 'win32' ? 'lumen.exe'
  : process.platform === 'darwin' ? 'Lumen.app/Contents/MacOS/lumen' : 'lumen';
const target = join(project, 'payload', executable);
mkdirSync(dirname(target), { recursive: true });
const tags = process.platform === 'linux' ? 'desktop,production,webkit2_41' : 'desktop,production';
const args = ['build', '-buildvcs=false', '-tags', tags, '-o', target];
if (process.platform === 'win32') args.push('-ldflags', '-H=windowsgui');
args.push('./cmd/lumen');
// Wails v2.15 references UTType on current SDKs without linking its framework.
const env = process.platform === 'darwin'
  ? { ...process.env, CGO_LDFLAGS: `${process.env.CGO_LDFLAGS ?? ''} -framework UniformTypeIdentifiers`.trim() }
  : process.env;
const result = spawnSync('go', args, { cwd: project, stdio: 'inherit', env });
if (result.error) throw result.error;
if (result.status !== 0) process.exit(result.status ?? 1);
if (process.platform === 'darwin') {
  writeFileSync(join(project, 'payload', 'Lumen.app', 'Contents', 'Info.plist'), `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>CFBundleExecutable</key><string>lumen</string>
<key>CFBundleIdentifier</key><string>io.example.lumen</string>
<key>CFBundleName</key><string>Lumen</string>
<key>CFBundlePackageType</key><string>APPL</string>
<key>CFBundleShortVersionString</key><string>1.0.0</string>
<key>NSHighResolutionCapable</key><true/>
</dict></plist>
`);
}
console.log(`Prepared ${target}`);
