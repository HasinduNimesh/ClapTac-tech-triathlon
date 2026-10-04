export type CatalogProduct = {
  id: string;
  family: string;
  name: string;
  size: string;
  pack: string;
  unitsPerPack: number;
  packWeightKg: number;
  packVolumeM3: number;
  temperature: "ambient" | "chilled";
};

export type DraftLine = {
  productId: string;
  name: string;
  pack: string;
  unitsPerPack: number;
  quantity: number;
  weightKg: number;
  volumeM3: number;
  temperature: string;
  sourceText: string;
};

export type PreviousOrder = {
  orderRef: string;
  placedOn: string;
  requestedDeliveryDate: string;
  temperatureRequirement: string;
  orderUnits: number;
  orderWeightKg: number;
  orderVolumeM3: number;
};

export type FormFill = {
  temperature: "ambient" | "chilled";
  orderUnits: number;
  orderWeightKg: number;
  orderVolumeM3: number;
  lines: DraftLine[];
  fromOrderRef: string;
};

export function lineFromProduct(product: CatalogProduct, quantity: number | string, sourceText?: string): DraftLine | null;
export function formFills(lines: DraftLine[], previousOrder: PreviousOrder | null, includePrevious: boolean): FormFill[];
