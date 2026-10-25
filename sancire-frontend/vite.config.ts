import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import path from "node:path";
import { fileURLToPath } from "node:url";

const root = path.dirname(fileURLToPath(import.meta.url));
export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: { alias: { "@": path.resolve(root, "src") } },
  server: {
    host: "0.0.0.0",
    allowedHosts: true,
    proxy: { "/api": { target: process.env.VITE_API_TARGET || "http://127.0.0.1:8080", changeOrigin: true } },
  },
  preview: { host: "0.0.0.0", allowedHosts: true },
});
