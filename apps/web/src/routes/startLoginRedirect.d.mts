export function startLoginRedirect(login: () => Promise<void>, onFailure: () => void, onStart?: () => void, onSuccess?: () => void): Promise<boolean>;
