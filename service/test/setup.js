import { env } from "cloudflare:workers";
import { applyD1Migrations } from "cloudflare:test";
import { beforeEach } from "vitest";

await applyD1Migrations(env.DB, env.TEST_MIGRATIONS);
beforeEach(async () => {
  await env.DB.prepare("DROP TRIGGER IF EXISTS fail_receipt").run();
  await env.DB.batch([
    ...[
      "import_items",
      "import_runs",
      "oauth_credentials",
      "oauth_grants",
      "oauth_pending",
      "oauth_clients",
    ].map((table) => env.DB.prepare("DELETE FROM " + table)),
    env.DB.prepare("DELETE FROM task_events"),
    env.DB.prepare("DELETE FROM operation_receipts"),
    env.DB.prepare("DELETE FROM tasks"),
    env.DB.prepare("DELETE FROM owners"),
  ]);
});
