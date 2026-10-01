export function calibratedArrivalRange(estimate: { eta?: string; kind?: string } | undefined, history: {
  arrivalRangeStatus?: string;
  arrivalOffsetP10Minutes?: number;
  arrivalOffsetP90Minutes?: number;
  arrivalRangeSamples?: number;
  arrivalRangeHoldouts?: number;
  arrivalRangeCoverage?: number;
  arrivalRangeVersion?: string;
} | undefined): { lower: Date; upper: Date; sampleCount: number; holdoutCount: number; coverage?: number; version?: string } | undefined;
