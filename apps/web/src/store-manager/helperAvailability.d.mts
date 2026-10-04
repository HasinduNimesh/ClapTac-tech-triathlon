export function helperStatusFrom(body: { orderAssistant?: unknown; dashboardAssistant?: unknown } | null | undefined): { order: boolean; dashboard: boolean };
export function isHelperUnavailable(error: unknown): boolean;
