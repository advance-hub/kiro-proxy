import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
  base: "/",
  clearScreen: false,
  server: {
    port: 1420,
    strictPort: true,
  },
  build: {
    target: "es2020",
    minify: "esbuild",
    cssTarget: "es2020",
    rollupOptions: {
      output: {
        // 去掉 crossorigin 属性，避免 WebView2 CORS 问题
        format: "es",
      },
    },
  },
  html: {
    cspNonce: undefined,
  },
});
