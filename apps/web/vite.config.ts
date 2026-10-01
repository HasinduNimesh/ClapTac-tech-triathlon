import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import { VitePWA } from "vite-plugin-pwa";

export default defineConfig({
  plugins: [
    react(),
    VitePWA({
      registerType: "autoUpdate",
      includeAssets: [],
      manifest: {
        name: "Waypoint Driver",
        short_name: "Waypoint",
        display: "standalone",
        start_url: "/driver/trips",
        background_color: "#f4f7f5",
        theme_color: "#0b6e4f",
      },
      workbox: {
        globPatterns: ["**/*.{js,css,html,ico,png,svg,woff2}"],
        navigateFallback: "/index.html",
        navigateFallbackDenylist: [/^\/api/, /^\/health/, /^\/oauth2/],
      },
    }),
  ],
  server: {
    port: 3000,
    host: true,
    proxy: {
      "/api": {
        target: "http://nginx",
        changeOrigin: true,
      },
    },
  },
});
