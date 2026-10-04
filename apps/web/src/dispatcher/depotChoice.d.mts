export function effectiveDepot(homeDepot: string | undefined, choice: string | undefined): string;
export function readDepotChoice(storage: Pick<Storage, "getItem"> | undefined, userId: string | undefined): string | undefined;
export function saveDepotChoice(storage: Pick<Storage, "setItem"> | undefined, userId: string | undefined, depot: string): void;
