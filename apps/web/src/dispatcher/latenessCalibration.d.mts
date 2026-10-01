export type CalibratedLatenessSegment = {
  brand: string;
  brierScore?: number | null;
  calibrationSampleCount: number;
  calibrationStatus: string;
  calibrationVersion: string;
};

export function evaluatedLatenessCalibrations<T extends CalibratedLatenessSegment>(history: readonly T[]): T[];
