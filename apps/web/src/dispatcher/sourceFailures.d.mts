export type SourceState = { error: string; status?: number; missingIsEmpty?: boolean };
export function isSourceFailure(source: SourceState | null | undefined): boolean;
export function failedSourceCount(sources: SourceState[]): number;
