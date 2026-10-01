const blobAsDataURL = async blob => {
  const bytes = new Uint8Array(await blob.arrayBuffer());
  let binary = "";
  const chunkSize = 0x8000;
  for (let offset = 0; offset < bytes.length; offset += chunkSize) {
    binary += String.fromCharCode(...bytes.subarray(offset, offset + chunkSize));
  }
  return `data:${blob.type || "application/octet-stream"};base64,${btoa(binary)}`;
};

const stripCredentialFields = value => {
  if (Array.isArray(value)) return value.map(stripCredentialFields);
  if (!value || typeof value !== "object" || value instanceof Blob) return value;
  return Object.fromEntries(Object.entries(value)
    .filter(([key]) => !/(token|authorization|secret|password)/i.test(key))
    .map(([key, nested]) => [key, stripCredentialFields(nested)]));
};

export async function createDriverDataExport({ subject, trips, details, queue }, exportedAt = new Date()) {
  if (typeof subject !== "string" || !subject.trim()) throw new Error("An authenticated driver account is required to export saved data.");
  const exportedQueue = await Promise.all(queue.map(async item => {
    const { blob, ...metadata } = item;
    return {
      ...stripCredentialFields(metadata),
      proofBlob: blob instanceof Blob ? {
        mimeType: blob.type || item.mimeType || "application/octet-stream",
        dataUrl: await blobAsDataURL(blob),
      } : undefined,
    };
  }));
  return JSON.stringify({
    format: "waypoint-driver-offline-export-v1",
    subject,
    exportedAt: exportedAt.toISOString(),
    trips,
    details,
    pendingOperations: exportedQueue,
  }, null, 2);
}

export function clearableCompletedTripIds({ pendingQueueCount, markers, details }) {
  if (!Number.isInteger(pendingQueueCount) || pendingQueueCount !== 0) return [];
  const byTrip = new Map(details.map(detail => [detail.tripId, detail]));
  return [...new Set(markers.flatMap(marker => {
    const prefix = "serverCompletedAt:";
    if (typeof marker.key !== "string" || !marker.key.startsWith(prefix)) return [];
    let tripId;
    try { tripId = decodeURIComponent(marker.key.slice(prefix.length)); }
    catch { return []; }
    const detail = byTrip.get(tripId);
    return detail?.run?.status === "completed" && detail.run.completedAt === marker.value ? [tripId] : [];
  }))];
}
