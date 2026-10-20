import { defineWranglerConfig } from "wrangler/experimental-config";

export default defineWranglerConfig({
	types: { generate: false },
	dev: { ip: "127.0.0.1", port: 8787 },
	build: {
		command: ".tools/bin/worker-build --release --no-panic-recovery",
		watchDir: [
			"rust",
			"Cargo.toml",
			"Cargo.lock",
		],
	},
});
