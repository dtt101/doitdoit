"use strict";
const fs = require("node:fs");
const { prepare } = require("../../web/record-migration.js");
(async () => {
  const raw = JSON.parse(fs.readFileSync(0, "utf8"));
  process.stdout.write(JSON.stringify(await Promise.all(raw.map(prepare))));
})().catch(error => { process.stderr.write(String(error)); process.exitCode = 1; });
