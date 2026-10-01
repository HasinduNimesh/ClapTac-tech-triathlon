/** Start OIDC login and report a redirect failure to the caller. */
export async function startLoginRedirect(login, onFailure, onStart = () => undefined, onSuccess = () => undefined) {
  onStart();
  try {
    await login();
    onSuccess();
    return true;
  } catch {
    onFailure();
    return false;
  }
}
