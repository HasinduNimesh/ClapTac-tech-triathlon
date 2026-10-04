export const INSTALL_INTRO_KEY: string;
export function installCardState(input?: { standalone?: boolean; dismissed?: boolean; canPrompt?: boolean }): "hidden" | "prompt" | "manual";
export function manualInstallSteps(userAgent?: string): string;
