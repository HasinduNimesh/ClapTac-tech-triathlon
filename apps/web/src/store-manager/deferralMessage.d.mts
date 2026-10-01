export type DeferralExplanation = { message: string; nextAction: string };
export const deferralMessages: Record<string, DeferralExplanation>;
export const genericDeferralMessage: DeferralExplanation;
export function deferralExplanation(reasonCode: string | undefined): DeferralExplanation;
