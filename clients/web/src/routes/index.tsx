import { createFileRoute } from "@tanstack/react-router";
import { useEffect } from "react";
import styled, { ThemeProvider } from "styled-components";
import { FaqPage } from "@/components/pages/faq-page";
import { JoinPage } from "@/components/pages/join-page";
import { SettingsPage } from "@/components/pages/settings-page";
import { AppShell, type KeyHint } from "@/components/shell/app-shell";
import GlobalStyle from "@/components/styles/global-style";
import { Terminal } from "@/components/terminal";
import { type PageId, ShellProvider, useShell } from "@/contexts/shell-context";
import { UserProvider } from "@/contexts/user-context";
import { type RoomParams, useRoomParams } from "@/hooks/use-room-params";
import { themeContext, useTheme } from "@/hooks/use-theme";

export const Route = createFileRoute("/")({
	component: App,
});

const pageHints: Record<PageId, KeyHint[]> = {
	terminal: [
		["tab", "complete"],
		["↑↓", "history"],
		["ctrl+l", "clear"],
	],
	join: [
		["enter", "send invite"],
		["esc", "clear"],
	],
	faq: [["enter", "expand"]],
	settings: [["enter", "select"]],
};

function App() {
	const [roomParams] = useRoomParams();
	const { theme, themeLoaded, setMode } = useTheme();

	// Keep the browser chrome colour in sync with the active theme.
	useEffect(() => {
		if (!themeLoaded) {
			return;
		}
		document
			.querySelector("meta[name='theme-color']")
			?.setAttribute("content", theme.colors.body);
	}, [theme, themeLoaded]);

	// An invite link with room codes lands straight on the join form.
	const initialPage: PageId = roomParams.joinCode ? "join" : "terminal";

	return (
		<UserProvider>
			<h1 className="sr-only">Visper Web</h1>

			{themeLoaded && (
				<ThemeProvider theme={theme}>
					<GlobalStyle />
					<themeContext.Provider value={setMode}>
						<ShellProvider initialPage={initialPage}>
							<Pages roomParams={roomParams} />
						</ShellProvider>
					</themeContext.Provider>
				</ThemeProvider>
			)}
		</UserProvider>
	);
}

function Pages({ roomParams }: { roomParams: RoomParams }) {
	const { page } = useShell();

	return (
		<AppShell hints={pageHints[page]}>
			{/* The terminal stays mounted so its scrollback survives tab switches. */}
			<TerminalSlot hidden={page !== "terminal"}>
				<Terminal searchParams={roomParams} active={page === "terminal"} />
			</TerminalSlot>
			{page === "join" && <JoinPage />}
			{page === "faq" && <FaqPage />}
			{page === "settings" && <SettingsPage />}
		</AppShell>
	);
}

const TerminalSlot = styled.div`
  display: flex;
  flex-direction: column;
  flex: 1;
  min-height: 0;

  &[hidden] {
    display: none;
  }
`;
