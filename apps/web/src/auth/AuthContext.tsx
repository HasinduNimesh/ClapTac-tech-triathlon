import { User } from "oidc-client-ts";
import { createContext, useCallback, useContext, useEffect, useRef, useState } from "react";
import { SESSION_REJECTED_EVENT } from "../api/client";
import { userManager } from "./userManager";
import { oidcConfig } from "./oidc";
import { canRenew, minutesLeft, msUntilChange, sessionPhase } from "./sessionClock.mjs";
import { rememberReturn } from "./returnTo.mjs";
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

export type SessionPhase = "active" | "expiring" | "expired";

type AuthState = {
  user: User | null;
  /** Where the sign-in is in its hour: warn while it can still be saved, say clearly when it has ended. */
  sessionPhase: SessionPhase;
  sessionMinutesLeft: number;
  /** Keeps the person signed in: renews quietly when the sign-in can renew itself, else signs in again and returns here. */
  renewSession: () => Promise<void>;
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
  sessionPhase: "active",
  sessionMinutesLeft: 0,
  renewSession: async () => undefined,
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
  const [clock, setClock] = useState(() => Date.now());
  const [rejected, setRejected] = useState(false);
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

  // The server refusing the sign-in (401) ends the session as far as the person is concerned, whatever the clock says.
  // One endpoint refusing a token can be that endpoint's own fault, so the sign-in itself is checked against the profile
  // endpoint before the person is told their session ended.
  useEffect(() => {
    if (!user?.access_token) return;
    const token = user.access_token;
    let checking = false;
    const onRejected = () => {
      if (checking) return;
      checking = true;
      fetch("/api/v1/shared/profiles/me", { headers: { Authorization: `Bearer ${token}` } })
        .then((res) => { if (res.status === 401) setRejected(true); })
        .catch(() => undefined)
        .finally(() => { window.setTimeout(() => { checking = false; }, 5000); });
    };
    window.addEventListener(SESSION_REJECTED_EVENT, onRejected);
    return () => window.removeEventListener(SESSION_REJECTED_EVENT, onRejected);
  }, [user?.access_token]);
  // A new sign-in (or a quiet renewal) clears an earlier refusal.
  useEffect(() => { setRejected(false); setClock(Date.now()); }, [user?.access_token]);

  const expiresAtMs = user?.expires_at ? user.expires_at * 1000 : NaN;
  // Wake exactly when the phase can change; while the warning shows, tick once a minute so "N minutes left" stays true.
  useEffect(() => {
    if (!user) return;
    const wait = msUntilChange(expiresAtMs, Date.now());
    if (wait === null) return;
    const phase = sessionPhase(expiresAtMs, Date.now());
    const timer = window.setTimeout(() => setClock(Date.now()), phase === "expiring" ? Math.min(wait, 60_000) : wait + 50);
    return () => window.clearTimeout(timer);
  }, [user, expiresAtMs, clock]);
  const phase: SessionPhase = user ? sessionPhase(expiresAtMs, clock, rejected) : "active";

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
        sessionPhase: phase,
        sessionMinutesLeft: user && Number.isFinite(expiresAtMs) ? minutesLeft(expiresAtMs, clock) : 0,
        renewSession: async () => {
          if (canRenew(oidcConfig().scope) && user?.refresh_token) {
            try { await userManager.signinSilent(); return; } catch { /* fall through to a full sign-in */ }
          }
          // A full sign-in leaves the page, so remember where to come back to.
          rememberReturn(window.sessionStorage, `${window.location.pathname}${window.location.search}`);
          await userManager.signinRedirect();
        },
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
