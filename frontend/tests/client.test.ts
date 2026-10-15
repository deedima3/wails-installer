import { describe, expect, it, vi } from 'vitest';
import { createClient, errorMessage } from '../src/lib/client.js';
import type { InstallerBindings, InstallerInfo, InstallerState, InstallResult } from '../src/lib/types.js';

describe('binding adapter', () => {
  it('forwards every method and install options without a Wails global', async () => {
    const bindings: InstallerBindings = {
      Info: vi.fn(async () => ({} as InstallerInfo)),
      State: vi.fn(async () => ({ status: 'ready', progress: {} as InstallerState['progress'], error: '' })),
      Browse: vi.fn(async () => '/apps/My app'),
      Install: vi.fn(async () => ({} as InstallResult)),
      Cancel: vi.fn(async () => {}), Launch: vi.fn(async () => {})
    };
    const client = createClient(bindings);
    const request = { directory: '/apps/My app', startup: true };
    await client.info(); await client.state(); expect(await client.browse()).toBe('/apps/My app');
    await client.install(request); await client.cancel(); await client.launch();
    for (const method of Object.values(bindings)) expect(method).toHaveBeenCalledOnce();
    expect(bindings.Install).toHaveBeenCalledWith(request);
  });
  it('preserves backend rejections', async () => {
    const client = createClient({ Install: async () => { throw 'destination exists'; } } as unknown as InstallerBindings);
    await expect(client.install({ directory: '/apps/Test', startup: false })).rejects.toBe('destination exists');
    expect(errorMessage('destination exists')).toBe('destination exists');
    expect(errorMessage(new Error('cancelled'))).toBe('cancelled');
  });
  it('normalizes generated Go pointer types and rejects unknown states', async () => {
    const State = vi.fn(async () => ({ status: 'ready', progress: {} as InstallerState['progress'], error: '' }));
    const client = createClient({ State } as unknown as InstallerBindings);
    expect((await client.state()).result).toBeNull();
    State.mockResolvedValueOnce({ status: 'unexpected', progress: {} as InstallerState['progress'], error: '' });
    await expect(client.state()).rejects.toThrow('Unknown installer status: unexpected');
  });
});
