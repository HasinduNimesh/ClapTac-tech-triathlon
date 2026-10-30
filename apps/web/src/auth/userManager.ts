import { UserManager, WebStorageStateStore } from "oidc-client-ts";
import { oidcConfig } from "./oidc";
import { canRenew } from "./sessionClock.mjs";
import { storageWithoutRefreshToken } from "./tokenStorage.mjs";

export function createUserManager(): UserManager {
  const config = oidcConfig();
  const renewable = canRenew(config.scope);
  // The identity server may send a refresh token even when this app never asks to renew. Nothing would use it, so it is
  // not written to the page's storage, where a script injected into the site could read it.
  const store = storageWithoutRefreshToken(window.sessionStorage, renewable);
  return new UserManager({
    authority: config.issuer,
    client_id: config.clientId,
    redirect_uri: config.redirectUri,
    response_type: "code",
    scope: config.scope,
    resource: config.resource,
    extraTokenParams: { resource: config.resource },
    // Renews with a refresh token shortly before the sign-in ends, but only when the sign-in was granted one.
    // Without offline_access there is nothing to renew with, and the session warning takes over.
    automaticSilentRenew: renewable,
    loadUserInfo: false,
    userStore: new WebStorageStateStore({ store }),
    stateStore: new WebStorageStateStore({ store: window.sessionStorage }),
  });
}

export const userManager = createUserManager();
