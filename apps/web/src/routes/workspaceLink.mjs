// Where "Open my workspace" goes for each role. The loader's workspace is a separate app, so it is a
// plain link, not a router link.
export function workspaceFor(role) {
  switch (String(role ?? "").toUpperCase()) {
    case "STORE_MANAGER": return { href: "/store-manager", external: false };
    case "DISPATCHER": return { href: "/dispatcher", external: false };
    case "LOADER": return { href: "/loader-app/", external: true };
    case "DRIVER": return { href: "/driver/trips", external: false };
    default: return { href: "/login", external: false };
  }
}
