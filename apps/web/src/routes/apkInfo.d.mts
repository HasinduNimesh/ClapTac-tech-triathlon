export function formatBytes(bytes: number): string;
export type ApkInfo = { version: string; sizeLabel: string; sha256: string; shaShort: string; builtOn: string };
export function describeApk(meta: unknown): ApkInfo | null;
