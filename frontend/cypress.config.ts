import { defineConfig } from "cypress";
import { Client } from "pg";

// Test-only: connects directly to Postgres to flip a user's tier, since
// there's no self-service upgrade flow yet (billing is roadmap 3.8,
// unimplemented). Run ./scripts/dev-postgres.sh to start a local Postgres
// container matching this default connection string.
const POSTGRES_URL =
  process.env.POSTGRES_URL ?? "postgres://dopad:dopad@localhost:5432/dopad?sslmode=disable";

export default defineConfig({
  e2e: {
    baseUrl: "http://localhost:3000",
    supportFile: false,
    env: {
      apiUrl: process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080",
    },
    setupNodeEvents(on) {
      on("task", {
        async setUserTier({ username, tier }: { username: string; tier: string }) {
          const client = new Client({ connectionString: POSTGRES_URL });
          await client.connect();
          try {
            await client.query("UPDATE users SET tier = $1 WHERE username = $2", [tier, username]);
          } finally {
            await client.end();
          }
          return null;
        },
      });
    },
  },
});
