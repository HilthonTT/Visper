import tailwindcss from "@tailwindcss/vite";
import { devtools } from "@tanstack/devtools-vite";
import { tanstackStart } from "@tanstack/react-start/plugin/vite";
import viteReact from "@vitejs/plugin-react";
import { nitro } from "nitro/vite";
import { defineConfig } from "vite";

export default defineConfig({
	resolve: {
		// Resolves the `@/*` alias declared in tsconfig.json.
		tsconfigPaths: true,
	},
	ssr: {
		// Bundle the ESM build: the CJS entry breaks the `styled.x` default
		// import during dev SSR ("styled.button is not a function").
		noExternal: ["styled-components"],
	},
	plugins: [devtools(), nitro(), tailwindcss(), tanstackStart(), viteReact()],
});
