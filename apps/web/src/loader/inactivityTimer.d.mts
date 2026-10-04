export const LOADER_INACTIVITY_MS: number;
export function createLoaderInactivityTimer(
  signOut: () => Promise<void>,
  clock?: Pick<typeof globalThis, "setTimeout" | "clearTimeout">,
): { reset: () => void; stop: () => void; switchUser: () => Promise<void> };
