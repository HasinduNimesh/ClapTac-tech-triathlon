export function singleFlight<Args extends unknown[], Result>(
  run: (...args: Args) => Promise<Result>,
  keyOf?: (...args: Args) => string,
): (...args: Args) => Promise<Result>;
