export function acquireBarcodeCamera(
  mediaDevices: Pick<MediaDevices, "getUserMedia"> | undefined,
  video: HTMLVideoElement | null,
  isActive?: () => boolean,
): Promise<(() => void) | undefined>;
