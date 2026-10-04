export const LOADER_INACTIVITY_MS = 5 * 60 * 1000;

export function createLoaderInactivityTimer(signOut, clock = globalThis) {
  let timer;
  let signingOut = false;

  function stop() {
    if (timer !== undefined) clock.clearTimeout(timer);
    timer = undefined;
  }

  function reset() {
    if (signingOut) return;
    stop();
    timer = clock.setTimeout(() => { void switchUser(); }, LOADER_INACTIVITY_MS);
  }

  async function switchUser() {
    if (signingOut) return;
    signingOut = true;
    stop();
    await signOut();
  }

  reset();
  return { reset, stop, switchUser };
}
