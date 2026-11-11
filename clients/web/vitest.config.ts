import { defineConfig } from "vitest/config";

// Kept separate from vite.config.ts so unit tests don't boot the
// TanStack Start / Nitro / devtools plugins.
export default defineConfig({
	resolve: {
		tsconfigPaths: true,
	},
	test: {
		environment: "node",
		include: ["src/**/*.test.ts"],
	},
});
