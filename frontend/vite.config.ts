import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import wails from "@wailsio/runtime/plugins/vite";
import path from "node:path";

// https://vitejs.dev/config/
export default defineConfig({
  server: {
    // Bind all stacks: wails3's dev asset server proxies "localhost", which
    // Windows resolves to ::1 first — an IPv4-only bind makes it fail with
    // "unable to connect to frontend server".
    host: true,
    port: Number(process.env.WAILS_VITE_PORT) || 9245,
    strictPort: true,
  },
  plugins: [react(), tailwindcss(), wails("./bindings")],
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./src"),
    },
  },
});
