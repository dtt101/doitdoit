// Cloudflare tooling configuration only; application code remains Rust.
import { bindings, defineConfig, triggers } from "cf/config";

export default defineConfig(({ mode }) => {
  const production = mode === "production";
  if (mode && mode !== "production" && mode !== "development") {
    throw new Error("Use development or production mode.");
  }
  return {
    accountId: "91d8cf0b8c805dac586f9ab55d6d6212",
    worker: {
      name: production ? "doitdoit-service" : "doitdoit-service-local",
      compatibilityDate: "2026-10-19",
      compatibilityFlags: ["global_fetch_strictly_public"],
      entrypoint: "build/worker/shim.mjs",
      workersDev: production,
      previewUrls: false,
      triggers: production ? [triggers.scheduled({ schedule: "17 * * * *" })] : [],
      observability: {
        enabled: production,
        redactQueryString: true,
        logs: { enabled: production, invocationLogs: false },
        traces: { enabled: production, headSamplingRate: 0.01 },
      },
      env: {
        SERVICE_ORIGIN: bindings.text(production
          ? "https://doitdoit-service.davidt-91d.workers.dev"
          : "http://localhost:8787"),
        BROWSER_ORIGIN: bindings.text(production ? "https://dtt101.github.io" : "http://localhost:8000"),
        OWNER_TIMEZONE: bindings.text("Europe/London"),
        GITHUB_CLIENT_ID: bindings.secret(),
        GITHUB_CLIENT_SECRET: bindings.secret(),
        GITHUB_OWNER_ID: bindings.secret(),
        DB: bindings.d1({
          name: production ? "doitdoit-prod" : "doitdoit-local",
          id: production ? "43659423-7b12-42ac-8f1e-fc97f3ed0fea"
            : "00000000-0000-0000-0000-000000000000",
        }),
        AUTH_LIMIT: bindings.rateLimit({ namespace: "2026101901", simple: { limit: 60, period: 60 } }),
        REGISTER_LIMIT: bindings.rateLimit({ namespace: "2026101902", simple: { limit: 10, period: 60 } }),
      },
    },
  };
});
