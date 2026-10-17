import path from "path"

import tailwindcss from "@tailwindcss/vite"
import { tanstackRouter } from "@tanstack/router-plugin/vite"
import react from "@vitejs/plugin-react"
import { defineConfig } from "vite"

// https://vite.dev/config/
export default defineConfig({
  plugins: [
    tanstackRouter({
      target: "react",
      autoCodeSplitting: true,
    }),
    react(),
    tailwindcss(),
  ],
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./src"),
    },
  },
  build: {
    chunkSizeWarningLimit: 2048,
  },
  server: {
    // The dashboard listens on a single port (see pkg/config/defaults.go).
    proxy: {
      "/api": {
        target: "http://localhost:18790",
        changeOrigin: true,
      },
      "/webchat/media": {
        target: "http://localhost:18790",
        changeOrigin: true,
      },
      "/webchat/ws": {
        target: "ws://localhost:18790",
        ws: true,
      },
    },
  },
})
