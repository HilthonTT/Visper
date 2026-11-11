import { TanStackDevtools } from "@tanstack/react-devtools";
import { createRootRoute, HeadContent, Scripts } from "@tanstack/react-router";
import { TanStackRouterDevtoolsPanel } from "@tanstack/react-router-devtools";
import { NuqsAdapter } from "nuqs/adapters/tanstack-router";
import type { ReactNode } from "react";
import appCss from "../styles.css?url";

export const Route = createRootRoute({
	head: () => ({
		meta: [
			{ charSet: "utf-8" },
			{ name: "viewport", content: "width=device-width, initial-scale=1" },
			{ title: "Visper" },
			{
				name: "description",
				content: "Visper - anonymous, encrypted real-time chat.",
			},
			{ name: "theme-color", content: "#10B981" },
		],
		links: [
			{ rel: "stylesheet", href: appCss },
			{ rel: "icon", type: "image/svg+xml", href: "/logo.svg" },
			{ rel: "icon", href: "/favicon.ico", sizes: "any" },
			{ rel: "apple-touch-icon", href: "/logo192.png" },
			{ rel: "manifest", href: "/manifest.json" },
			{ rel: "preconnect", href: "https://fonts.googleapis.com" },
			{
				rel: "preconnect",
				href: "https://fonts.gstatic.com",
				crossOrigin: "anonymous",
			},
			{
				rel: "stylesheet",
				href: "https://fonts.googleapis.com/css2?family=IBM+Plex+Mono:wght@500;700&display=swap",
			},
		],
	}),

	shellComponent: RootDocument,
});

function RootDocument({ children }: { children: ReactNode }) {
	return (
		<html lang="en">
			<head>
				<HeadContent />
			</head>
			<body>
				<NuqsAdapter>
					{children}
					<TanStackDevtools
						config={{ position: "bottom-right" }}
						plugins={[
							{
								name: "TanStack Router",
								render: <TanStackRouterDevtoolsPanel />,
							},
						]}
					/>
					<Scripts />
				</NuqsAdapter>
			</body>
		</html>
	);
}
