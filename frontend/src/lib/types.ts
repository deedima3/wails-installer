export interface InstallerInfo {
  id: string;
  name: string;
  version: string;
  description: string;
  defaultDirectory: string;
  allowStartup: boolean;
  platform: string;
}

export interface InstallRequest { directory: string; startup: boolean }
export interface InstallResult { directory: string; executable: string; startup: boolean }
export interface Progress {
  phase: string;
  file: string;
  copiedBytes: number;
  totalBytes: number;
  percent: number;
}
export interface InstallerState {
  status: 'ready' | 'installing' | 'complete' | 'error' | 'cancelled';
  progress: Progress;
  error: string;
  result: InstallResult | null;
}

/** Wails generates Go string fields as string and pointers as optional values. */
export interface BindingState {
  status: string;
  progress: Progress;
  error: string;
  result?: InstallResult | null;
}

/** Pass generated Go functions rather than relying on window.go or import paths. */
export interface InstallerBindings {
  Info(): Promise<InstallerInfo>;
  State(): Promise<BindingState>;
  Browse(): Promise<string>;
  Install(request: InstallRequest): Promise<InstallResult>;
  Cancel(): Promise<void>;
  Launch(): Promise<void>;
}

export interface InstallerClient {
  info(): Promise<InstallerInfo>;
  state(): Promise<InstallerState>;
  browse(): Promise<string>;
  install(request: InstallRequest): Promise<InstallResult>;
  cancel(): Promise<void>;
  launch(): Promise<void>;
}
