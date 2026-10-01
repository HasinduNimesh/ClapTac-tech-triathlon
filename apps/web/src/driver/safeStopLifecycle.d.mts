export function installSafeStopLock(options: {
  document: Pick<Document, "visibilityState" | "addEventListener" | "removeEventListener">;
  window: Pick<Window, "addEventListener" | "removeEventListener">;
  onLock: (locked: false) => void;
}): () => void;
