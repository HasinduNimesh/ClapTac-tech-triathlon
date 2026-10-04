export type Prefill = { outletId: string; orderUnits: number; orderWeightKg: number; orderVolumeM3: number; temperatureRequirement: "ambient" | "chilled" };
export type Definition = { version: number; name: string; weekday: number; time: string; timezone: string; action: "notify_deferrals" | "prefill_order" | "priority_review"; prefill?: Prefill };
export type Habit = { id: string; pattern: string; action: Definition["action"]; evidence: string; prefill?: Prefill; outletId?: string; yesCount: number; noCount: number };
export type Result = { at: string; message: string; items: { orderId: string; outletId: string; date: string; count: number }[]; prefill?: Prefill };
export const days = ["Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"];
export const apiBase = "/shared/automations";
export function fromHabit(h: Habit): Definition {
  const today = new Date().toLocaleDateString("en-US", { timeZone: "Asia/Colombo", weekday: "long" });
  return { version: 1, name: h.action === "prefill_order" ? "Prepare my regular order" : "Repeat-deferral review", weekday: days.indexOf(today), time: "09:00", timezone: "Asia/Colombo", action: h.action, ...(h.prefill ? { prefill: h.prefill } : {}) };
}
