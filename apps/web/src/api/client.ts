const base = import.meta.env.VITE_API_BASE_URL || "/api/v1";

export class ApiError extends Error {
  constructor(
    public status: number,
    message: string,
  ) {
    super(message);
  }
}

export async function apiJSON<T>(path: string, token: string, init?: RequestInit): Promise<T> {
  const res = await apiFetch(path, token, {
    ...init,
    headers: { "Content-Type": "application/json", ...(init?.headers || {}) },
  });
  const text = await res.text();
  if (!res.ok) {
    throw new ApiError(res.status, text || res.statusText);
  }
  return text ? (JSON.parse(text) as T) : ({} as T);
}

export async function apiFetch(path: string, token: string, init?: RequestInit): Promise<Response> {
  return fetch(`${base}${path}`, {
    ...init,
    headers: { Accept: "application/json", Authorization: `Bearer ${token}`, ...(init?.headers || {}) },
  });
}

export type OrderLine = {
  lineNo: number;
  productId: string;
  productName: string;
  pack: string;
  unitsPerPack: number;
  packQty: number;
  weightKg: number;
  volumeM3: number;
  source: string;
};

export type Order = {
  id: string;
  orderRef: string;
  outletId: string;
  brand: string;
  requestedDeliveryDate: string;
  orderUnits: number;
  orderWeightKg: number;
  orderVolumeM3: number;
  temperatureRequirement: string;
  status: string;
  /** The items on the order. Older and imported orders only have totals, so this can be missing. */
  lines?: OrderLine[];
};
