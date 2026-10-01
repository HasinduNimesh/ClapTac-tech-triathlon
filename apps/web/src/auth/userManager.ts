import { UserManager, WebStorageStateStore } from "oidc-client-ts";
import { oidcConfig } from "./oidc";

export function createUserManager(): UserManager {
  const config = oidcConfig();
  return new UserManager({
    authority: config.issuer,
    client_id: config.clientId,
    redirect_uri: config.redirectUri,
    response_type: "code",
    scope: "openid profile",
    automaticSilentRenew: false,
    loadUserInfo: false,
    userStore: new WebStorageStateStore({ store: window.sessionStorage }),
    stateStore: new WebStorageStateStore({ store: window.sessionStorage }),
  });
}

export const userManager = createUserManager();
