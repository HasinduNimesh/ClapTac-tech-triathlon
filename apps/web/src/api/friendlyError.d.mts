export const NETWORK_MESSAGE: string;
export function readableDetail(body: string): string;
export function friendlyMessage(status: number, body?: string): string;
export function describeError(error: unknown): string;
