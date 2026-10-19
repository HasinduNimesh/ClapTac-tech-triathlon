export const LOADER_HANDOFF_KEY: string;
export function markLoaderHandoff(storage?: Pick<Storage, "setItem">, now?: number): boolean;
