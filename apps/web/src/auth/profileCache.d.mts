export const PROFILE_CACHE_MAX_AGE_MS: number;

type CachedProfile = {
  userId: string;
  subject: string;
  displayName?: string;
  roles: string[];
  outletIds?: string[];
  depot?: string;
  vehicleId?: string;
};

export function clearCachedProfile(storage: Storage | undefined): void;
export function saveCachedProfile(storage: Storage | undefined, subject: string, profile: CachedProfile, now?: number): boolean;
export function loadAuthenticatedProfile(args: {
  fetchImpl: typeof fetch;
  storage: Storage | undefined;
  subject: string | undefined;
  accessToken: string;
  now?: number;
}): Promise<CachedProfile | null>;
