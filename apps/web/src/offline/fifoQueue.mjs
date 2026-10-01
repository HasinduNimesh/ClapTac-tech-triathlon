/** Apply queued work in order, removing an item only after it is accepted. */
export async function drainFIFOQueue({ list, apply, remove, onFailure }) {
  while (true) {
    const items = await list();
    const item = items[0];
    if (!item) return { kind: "ok", text: "Synced" };
    if (!item.id) {
      return { kind: "error", text: "A saved sync item is missing its local queue ID. The remaining queue is preserved." };
    }

    try {
      const result = await apply(item);
      if (!result.applied) {
        return {
          kind: "error",
          queueItemId: item.id,
          text: `Sync needs attention (${item.type}, ${item.operationId}): ${result.detail || "operation was not applied"}. ${items.length} item(s) remain saved on this device.`,
        };
      }
      await remove(item.id);
    } catch (error) {
      return onFailure(error, item, items.length);
    }
  }
}
