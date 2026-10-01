export interface OwnerStore {
  get(): Promise<string>;
  set(value: string): Promise<void>;
}
export function bindSingleOwner(store: OwnerStore, requestedOwner: string): Promise<boolean>;
