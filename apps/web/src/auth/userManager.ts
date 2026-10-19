import { UserManager, WebStorageStateStore } from "oidc-client-ts";
import { oidcConfig } from "./oidc";
import { canRenew } from "./sessionClock.mjs";

export function createUserManager(): UserManager {
  const config = oidcConfig();
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
    automaticSilentRenew: canRenew(config.scope),
    loadUserInfo: false,
    userStore: new WebStorageStateStore({ store: window.sessionStorage }),
    stateStore: new WebStorageStateStore({ store: window.sessionStorage }),
  });
}

export const userManager = createUserManager();
