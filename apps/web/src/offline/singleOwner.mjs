/** Bind one local driver database to one authenticated driver identity. */
export async function bindSingleOwner(store, requestedOwner) {
  if (typeof requestedOwner !== "string" || !requestedOwner.trim()) return false;
  const owner = await store.get();
  if (owner) return owner === requestedOwner;
  await store.set(requestedOwner);
  return (await store.get()) === requestedOwner;
}
