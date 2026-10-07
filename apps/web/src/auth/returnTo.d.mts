export const RETURN_KEY: string;
export function rememberReturn(storage: Pick<Storage, "setItem">, path: string): void;
export function takeReturn(storage: Pick<Storage, "getItem" | "removeItem">, role: string | undefined): string | null;
