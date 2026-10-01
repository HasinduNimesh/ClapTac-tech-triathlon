export function createSingleFlightAction(): (action: () => Promise<void>) => Promise<boolean>;
