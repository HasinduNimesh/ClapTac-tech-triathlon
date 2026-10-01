export function createSingleFlightAction() {
  let active = false;
  return async function run(action) {
    if (active) return false;
    active = true;
    try {
      await action();
      return true;
    } finally {
      active = false;
    }
  };
}
