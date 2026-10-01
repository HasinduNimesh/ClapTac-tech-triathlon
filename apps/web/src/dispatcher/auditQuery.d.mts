export type AuditFilters = {
  query: string;
  action: string;
  resourceType: string;
  resourceId: string;
  actorId: string;
  from: string;
  to: string;
};
export function buildAuditSearchParams(filters: AuditFilters, offset?: number): URLSearchParams;
