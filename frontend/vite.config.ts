import { defineConfig } from "vite";

// Base "./" so the built assets resolve from the embedded file:// origin
// that Wails serves after `wails build`.
export default defineConfig({
  base: "./",
  build: {
    outDir: "dist",
    emptyOutDir: true,
    target: "es2021",
    sourcemap: false,
  },
  server: {
    port: 5173,
    strictPort: true,
  },
});
