import { env } from "cloudflare:workers";
import { beforeEach, describe, expect, it } from "vitest";
import { RustTasks } from "./client.js";
import { createExecutionContext } from "cloudflare:test";

const principal = {
  ownerID: "github-12345",
  clientID: "test-client",
  scopes: ["tasks:read", "tasks:write"],
};
const create = (title = "Task") => ({
  operation_id: crypto.randomUUID(),
  title,
});
let tasks;
beforeEach(async () => {
  await env.DB.prepare(
    "INSERT INTO owners(id,timezone) VALUES(?,?) ON CONFLICT DO NOTHING",
  )
    .bind(principal.ownerID, "Europe/London")
    .run();
  tasks = new RustTasks(env, principal, createExecutionContext());
});
async function count(table) {
  return (await env.DB.prepare("SELECT count(*) AS n FROM " + table).first()).n;
}

describe("D1 atomic task operations", () => {
  it("preserves dates and notes, including explicit past dates and null", async () => {
    const task = await tasks.create({
      ...create(" Work "),
      notes: "☕\n  notes",
      scheduled_date: "2020-02-29",
    });
    expect(task).toMatchObject({
      title: "Work",
      notes: "☕\n  notes",
      scheduled_date: "2020-02-29",
      revision: 1,
    });
    await tasks.list();
    expect((await tasks.get(task.id)).revision).toBe(1);
    const completed = await tasks.update(task.id, {
      operation_id: crypto.randomUUID(),
      expected_revision: 1,
      changes: { status: "completed" },
    });
    expect(completed.scheduled_date).toBe("2020-02-29");
    expect(completed.completed_at).not.toBeNull();
    const cleared = await tasks.update(task.id, {
      operation_id: crypto.randomUUID(),
      expected_revision: 2,
      changes: { scheduled_date: null, status: "open" },
    });
    expect(cleared).toMatchObject({
      scheduled_date: null,
      status: "open",
      completed_at: null,
    });
    expect(await count("task_events")).toBe(3);
  });

  it("allows only one of two competing writes and leaves no losing receipt/event", async () => {
    const task = await tasks.create(create());
    const results = await Promise.allSettled(
      ["2030-01-01", "2030-01-02"].map((date) =>
        tasks.update(task.id, {
          operation_id: crypto.randomUUID(),
          expected_revision: 1,
          changes: { scheduled_date: date },
        }),
      ),
    );
    expect(results.filter((r) => r.status === "fulfilled")).toHaveLength(1);
    const failure = results.find((r) => r.status === "rejected");
    expect(failure.reason).toMatchObject({
      status: 409,
      code: "revision_conflict",
    });
    expect((await tasks.get(task.id)).revision).toBe(2);
    expect(await count("operation_receipts")).toBe(2);
    expect(await count("task_events")).toBe(2);
  });

  it("updates unrelated tasks independently", async () => {
    const a = await tasks.create(create("A"));
    const b = await tasks.create(create("B"));
    await Promise.all(
      [a, b].map((task) =>
        tasks.update(task.id, {
          operation_id: crypto.randomUUID(),
          expected_revision: 1,
          changes: { notes: "changed" },
        }),
      ),
    );
    expect((await tasks.get(a.id)).revision).toBe(2);
    expect((await tasks.get(b.id)).revision).toBe(2);
  });

  it("returns the original acknowledgement on sequential and concurrent retries", async () => {
    const command = create();
    const [a, b] = await Promise.all([
      tasks.create(command),
      tasks.create(command),
    ]);
    expect(a).toEqual(b);
    await tasks.update(a.id, {
      operation_id: crypto.randomUUID(),
      expected_revision: 1,
      changes: { title: "newer" },
    });
    expect(await tasks.create(command)).toEqual(a);
    expect(await count("tasks")).toBe(1);
    expect(await count("operation_receipts")).toBe(2);
    await expect(
      tasks.create({ ...command, title: "different" }),
    ).rejects.toMatchObject({ code: "operation_id_reused" });
  });

  it("rejects racing operation ID reuse with a different payload", async () => {
    const command = create();
    const results = await Promise.allSettled([
      tasks.create(command),
      tasks.create({ ...command, title: "other" }),
    ]);
    expect(results.filter((r) => r.status === "fulfilled")).toHaveLength(1);
    expect(await count("tasks")).toBe(1);
    expect(await count("task_events")).toBe(1);
  });

  it("retries an update after its revision has advanced without applying it twice", async () => {
    const task = await tasks.create(create());
    const update = {
      operation_id: crypto.randomUUID(),
      expected_revision: 1,
      changes: { notes: "edit" },
    };
    const a = await tasks.update(task.id, update);
    expect(await tasks.update(task.id, update)).toEqual(a);
    expect(await count("task_events")).toBe(2);
  });

  it("rolls back the task and event if failure occurs when storing the receipt result", async () => {
    const task = await tasks.create(create());
    await env.DB.prepare(
      "CREATE TRIGGER fail_receipt BEFORE UPDATE ON operation_receipts BEGIN SELECT RAISE(ABORT,'injected'); END",
    ).run();
    await expect(
      tasks.update(task.id, {
        operation_id: crypto.randomUUID(),
        expected_revision: 1,
        changes: { title: "must roll back" },
      }),
    ).rejects.toMatchObject({ status: 500, code: "internal_error" });
    expect(await tasks.get(task.id)).toEqual(task);
    expect(await count("task_events")).toBe(1);
    expect(await count("operation_receipts")).toBe(1);
  });

  it("isolates owners and enforces scopes", async () => {
    const task = await tasks.create(create());
    const other = new RustTasks(
      env,
      { ...principal, ownerID: "github-999" },
      createExecutionContext(),
    );
    await expect(other.list()).rejects.toMatchObject({ status: 403 });
    await expect(other.get(task.id)).rejects.toMatchObject({ status: 403 });
    const readonly = new RustTasks(
      env,
      {
        ...principal,
        scopes: ["tasks:read"],
      },
      createExecutionContext(),
    );
    await expect(readonly.create(create())).rejects.toMatchObject({
      status: 403,
    });
    expect(await count("tasks")).toBe(1);
  });

  it.each(["2025-02-29", "2026-04-31", "2026-1-01", "tomorrow"])(
    "rejects invalid date %s without writing",
    async (date) => {
      await expect(
        tasks.create({ ...create(), scheduled_date: date }),
      ).rejects.toMatchObject({ status: 400 });
      expect(await count("tasks")).toBe(0);
    },
  );
});

it("rejects forged identity headers", async () => {
  const { request } = await import("./client.js");
  const response = await request("/v1/tasks", {
    headers: {
      "X-Owner-ID": "github-12345",
      AUTHENTICATED_PRINCIPAL: JSON.stringify(principal),
    },
  });
  expect(response.status).toBe(401);
});
