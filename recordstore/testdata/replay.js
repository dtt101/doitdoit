// Cross-client test bridge only; not loaded by the static application.
"use strict";
const { replay } = require("../../web/record-replay.js");
const fs = require("node:fs");
(async () => {
  const cases = JSON.parse(fs.readFileSync(0, "utf8"));
  const views = [];
  for (const c of cases) views.push(await replay(c.records, c.backups));
  process.stdout.write(JSON.stringify(views));
})().catch(error => { process.stderr.write(String(error)); process.exitCode = 1; });
