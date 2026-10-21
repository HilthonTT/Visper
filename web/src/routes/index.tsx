import { createFileRoute } from "@tanstack/react-router";
import { useCallback, useEffect } from "react";
import { type DefaultTheme, ThemeProvider } from "styled-components";
import GlobalStyle from "@/components/styles/global-style";
import { Terminal } from "@/components/terminal";
import { UserProvider } from "@/contexts/user-context";
import { useRoomParams } from "@/hooks/use-room-params";
import { themeContext, useTheme } from "@/hooks/use-theme";

export const Route = createFileRoute("/")({
	component: App,
});

function App() {
	const [roomParams] = useRoomParams();
	const { theme, themeLoaded, setMode } = useTheme();

	// Stop arrow keys from scrolling the page; they navigate command history.
	useEffect(() => {
		const onKeyDown = (e: KeyboardEvent) => {
			if (e.code === "ArrowUp" || e.code === "ArrowDown") {
				e.preventDefault();
			}
		};
		window.addEventListener("keydown", onKeyDown);
		return () => window.removeEventListener("keydown", onKeyDown);
	}, []);

	// Keep the browser chrome colour in sync with the active theme.
	useEffect(() => {
		if (!themeLoaded) {
			return;
		}
		document
			.querySelector("meta[name='theme-color']")
			?.setAttribute("content", theme.colors.body);
	}, [theme, themeLoaded]);

	const themeSwitcher = useCallback(
		(switchTheme: DefaultTheme) => setMode(switchTheme),
		[setMode],
	);

	return (
		<UserProvider>
			<h1 className="sr-only">Visper Web</h1>

			{themeLoaded && (
				<ThemeProvider theme={theme}>
					<GlobalStyle />
					<themeContext.Provider value={themeSwitcher}>
						<Terminal searchParams={roomParams} />
					</themeContext.Provider>
				</ThemeProvider>
			)}
		</UserProvider>
	);
}
