import { User } from "oidc-client-ts";
import { createContext, useCallback, useContext, useEffect, useRef, useState } from "react";
import { userManager } from "./userManager";
import { clearCachedProfile, loadAuthenticatedProfile, saveCachedProfile } from "./profileCache.mjs";

type Profile = {
  userId: string;
  subject: string;
  displayName?: string;
  roles: string[];
  outletIds?: string[];
  depot?: string;
  vehicleId?: string;
};

type AuthState = {
  user: User | null;
  profile: Profile | null;
  profileLoading: boolean;
  login: () => Promise<void>;
  completeLogin: (user: User) => void;
  logout: () => Promise<void>;
  updateDisplayName: (name: string) => Promise<void>;
};

function profileStorage(): Storage | undefined {
  try { return window.localStorage; } catch { return undefined; }
}

const AuthContext = createContext<AuthState>({
  user: null,
  profile: null,
  profileLoading: true,
  login: async () => undefined,
  completeLogin: () => undefined,
  logout: async () => undefined,
  updateDisplayName: async () => undefined,
});

export function AuthProvider({ children }: { children: React.ReactNode }) {
  const [user, setUser] = useState<User | null>(null);
  const [profile, setProfile] = useState<Profile | null>(null);
  const [profileLoading, setProfileLoading] = useState(true);
  const authRevision = useRef(0);
  const completeLogin = useCallback((authenticatedUser: User) => {
    authRevision.current += 1;
    setUser(authenticatedUser);
  }, []);

  useEffect(() => {
    const revision = authRevision.current;
    userManager.getUser()
      .then((u) => { if (authRevision.current === revision) setUser(u && !u.expired ? u : null); })
      .catch(() => { if (authRevision.current === revision) setUser(null); })
      .finally(() => {
        if (authRevision.current === revision) {
          setProfileLoading(false);
        }
      });
    return userManager.events.addUserLoaded((u) => {
      authRevision.current += 1;
      setUser(u);
    });
  }, []);

  useEffect(() => {
    if (!user?.access_token) {
      clearCachedProfile(profileStorage());
      setProfile(null);
      setProfileLoading(false);
      return;
    }
    let active = true;
    setProfileLoading(true);
    const subject = user.profile.sub;
    loadAuthenticatedProfile({
      fetchImpl: fetch,
      storage: profileStorage(),
      subject,
      accessToken: user.access_token,
    })
      .then((loadedProfile) => { if (active) setProfile(loadedProfile); })
      .finally(() => { if (active) setProfileLoading(false); });
    return () => { active = false; };
  }, [user]);

  return (
    <AuthContext.Provider
      value={{
        user,
        profile,
        profileLoading,
        login: () => userManager.signinRedirect(),
        completeLogin,
        updateDisplayName: async (name) => {
          if (!user?.access_token) throw new Error("Sign in before changing your name.");
          const response = await fetch("/api/v1/shared/profiles/me/display-name", {
            method: "PUT",
            headers: { Authorization: `Bearer ${user.access_token}`, "Content-Type": "application/json" },
            body: JSON.stringify({ displayName: name }),
          });
          if (!response.ok) throw new Error("Could not save your name.");
          const body = await response.json() as { profile: Profile };
          if (body.profile?.subject !== user.profile.sub) throw new Error("Invalid profile response.");
          setProfile(body.profile);
          saveCachedProfile(profileStorage(), user.profile.sub, body.profile);
        },
        logout: async () => {
          authRevision.current += 1;
          await userManager.removeUser();
          clearCachedProfile(profileStorage());
          setUser(null);
          setProfile(null);
        },
      }}
    >
      {children}
    </AuthContext.Provider>
  );
}

export function useAuth() {
  return useContext(AuthContext);
}

export function roleOf(profile: Profile | null): string | undefined {
  return profile?.roles?.[0];
}
