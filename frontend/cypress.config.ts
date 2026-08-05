import { defineConfig } from "cypress";

export default defineConfig({
  e2e: {
    baseUrl: "http://localhost:3000",
    supportFile: false,
    env: {
      apiUrl: process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080",
    },
  },
});
