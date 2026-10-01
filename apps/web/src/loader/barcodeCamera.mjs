const CAMERA_CONSTRAINTS = { video: { facingMode: { ideal: "environment" } }, audio: false };

/** Acquire the rear-facing barcode camera and return an idempotent cleanup function. */
export async function acquireBarcodeCamera(mediaDevices, video, isActive = () => true) {
  if (typeof mediaDevices?.getUserMedia !== "function") {
    throw new Error("camera_unavailable");
  }

  const stream = await mediaDevices.getUserMedia(CAMERA_CONSTRAINTS);
  let stopped = false;
  const stop = () => {
    if (stopped) return;
    stopped = true;
    stream.getTracks().forEach(track => track.stop());
    if (video?.srcObject === stream) video.srcObject = null;
  };

  if (!video || !isActive()) {
    stop();
    return undefined;
  }

  try {
    video.srcObject = stream;
    await video.play();
  } catch (error) {
    stop();
    throw error;
  }

  if (!isActive()) {
    stop();
    return undefined;
  }
  return stop;
}
