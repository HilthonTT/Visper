import { type ReactNode, useEffect, useState } from "react";
import styled, { keyframes } from "styled-components";
import { type PageId, pages, useShell } from "@/contexts/shell-context";
import { checkHealth } from "@/lib/api";
import { getCompanion } from "@/lib/companions";

/*
 * Same frame as the CLI (clients/cli/internal/tui): a header with the logo,
 * page tabs and connection status; the page in the middle, with the
 * companion panel on the right on wide screens; and a footer with key hints.
 *
 *   visper▌  Terminal  j Join room  f FAQ  s Settings            ● online
 *   ─────────────────────────────────────────────────────────────────────
 *   page                                                   │ companion
 *   ─────────────────────────────────────────────────────────────────────
 *   tab complete · alt+1-4 page · alt+w companion       ● e2e encrypted
 */

export type KeyHint = readonly [key: string, desc: string];

const globalHints: KeyHint[] = [
	["alt+1-4", "page"],
	["alt+w", "companion"],
];

const HEALTH_INTERVAL_MS = 30_000;

/** Polls the API's `/health` endpoint; `null` until the first answer. */
function useApiOnline(): boolean | null {
	const [online, setOnline] = useState<boolean | null>(null);

	useEffect(() => {
		const controller = new AbortController();
		const poll = () =>
			checkHealth(controller.signal).then((ok) => {
				if (!controller.signal.aborted) {
					setOnline(ok);
				}
			});
		poll();
		const id = window.setInterval(poll, HEALTH_INTERVAL_MS);
		return () => {
			controller.abort();
			window.clearInterval(id);
		};
	}, []);

	return online;
}

/** True while focus is in a field that needs plain letter keys. */
function isTyping(target: EventTarget | null): boolean {
	return (
		target instanceof HTMLElement &&
		(target.isContentEditable ||
			["INPUT", "TEXTAREA", "SELECT"].includes(target.tagName))
	);
}

/** Page shortcuts: alt+1..4 anywhere, and the CLI's letters when not typing. */
function useShellKeys() {
	const { setPage, toggleCompanion } = useShell();

	useEffect(() => {
		const onKeyDown = (e: KeyboardEvent) => {
			if (e.ctrlKey || e.metaKey) {
				return;
			}
			if (e.altKey) {
				const n = Number(e.code.replace("Digit", ""));
				if (n >= 1 && n <= pages.length) {
					e.preventDefault();
					setPage(pages[n - 1].id);
				} else if (e.code === "KeyW") {
					e.preventDefault();
					toggleCompanion();
				}
				return;
			}
			if (isTyping(e.target)) {
				return;
			}
			const page = pages.find((p) => p.key === e.key.toLowerCase());
			if (page) {
				e.preventDefault();
				setPage(page.id);
			}
		};
		window.addEventListener("keydown", onKeyDown);
		return () => window.removeEventListener("keydown", onKeyDown);
	}, [setPage, toggleCompanion]);
}

interface AppShellProps {
	/** Key hints for the active page, shown before the global ones. */
	hints: KeyHint[];
	children: ReactNode;
}

export const AppShell = ({ hints, children }: AppShellProps) => {
	const { page, setPage, companionHidden } = useShell();
	const online = useApiOnline();
	useShellKeys();

	return (
		<Frame>
			<Header>
				<Logo aria-label="Visper">
					visper
					<Cursor aria-hidden="true" />
				</Logo>
				<Tabs role="tablist" aria-label="Pages">
					{pages.map((p, i) => (
						<Tab
							key={p.id}
							type="button"
							role="tab"
							aria-selected={page === p.id}
							title={`${p.label} (${p.key} or alt+${i + 1})`}
							onClick={() => setPage(p.id as PageId)}
						>
							{page !== p.id && <TabKey>{p.key}</TabKey>}
							{p.label}
						</Tab>
					))}
				</Tabs>
				<Status $online={online}>
					<span>{online ? "●" : "○"}</span>{" "}
					{online === null ? "connecting" : online ? "online" : "offline"}
				</Status>
			</Header>

			<Body>
				<Main role="tabpanel">{children}</Main>
				{/* Settings shows its own, larger preview. */}
				{!companionHidden && page !== "settings" && <CompanionPanel />}
			</Body>

			<Footer>
				<Hints>
					{hints.map(([key, desc]) => (
						<Hint key={key}>
							<HintKey>{key}</HintKey> {desc}
						</Hint>
					))}
					{globalHints.map(([key, desc]) => (
						<Hint key={key} $global>
							<HintKey>{key}</HintKey> {desc}
						</Hint>
					))}
				</Hints>
				<Badge>e2e encrypted</Badge>
			</Footer>
		</Frame>
	);
};

const CompanionPanel = () => {
	const { companionId, mood } = useShell();
	const companion = getCompanion(companionId);

	return (
		<Companion aria-label="Companion">
			<CompanionImage src={companion.src} alt={companion.title} />
			<Mood aria-live="polite">{mood}</Mood>
			<Subtle>alt+w hide</Subtle>
		</Companion>
	);
};

/* ===== Styles ===== */

const MOBILE = "@media (max-width: 640px)";

const Frame = styled.div`
  display: flex;
  flex-direction: column;
  height: 100dvh;
  max-width: 1320px;
  margin: 0 auto;
  padding: 0 1.75rem;

  ${MOBILE} {
    padding: 0 1rem;
  }
`;

const Header = styled.header`
  display: flex;
  align-items: center;
  gap: 1.5rem;
  padding: 1.1rem 0 0.8rem;
  border-bottom: 1px solid ${({ theme }) => theme.colors.border};

  ${MOBILE} {
    gap: 0.75rem;
  }
`;

const blink = keyframes`
  50% { opacity: 0; }
`;

const Logo = styled.div`
  display: flex;
  align-items: center;
  flex: none;
  font-weight: 700;
  background: linear-gradient(
    90deg,
    ${({ theme }) => theme.colors.primary},
    ${({ theme }) => theme.colors.secondary}
  );
  background-clip: text;
  -webkit-background-clip: text;
  color: transparent;
`;

const Cursor = styled.span`
  width: 0.55em;
  height: 1.1em;
  margin-left: 2px;
  background: ${({ theme }) => theme.colors.secondary};
  animation: ${blink} 1.1s steps(1) infinite;
`;

const Tabs = styled.nav`
  display: flex;
  gap: 0.25rem;
  min-width: 0;
  overflow-x: auto;
  scrollbar-width: none;

  &::-webkit-scrollbar {
    display: none;
  }
`;

const Tab = styled.button`
  flex: none;
  padding: 0 0.6rem;
  border-radius: 3px;
  color: ${({ theme }) => theme.colors.text[200]};
  white-space: nowrap;

  &:hover {
    color: ${({ theme }) => theme.colors.text[100]};
  }

  &[aria-selected="true"] {
    background: ${({ theme }) => theme.colors.primary};
    color: ${({ theme }) => theme.colors.onPrimary};
    font-weight: 700;
  }
`;

const TabKey = styled.span`
  margin-right: 0.5rem;
  color: ${({ theme }) => theme.colors.text[300]};
`;

const Status = styled.span<{ $online: boolean | null }>`
  margin-left: auto;
  flex: none;
  color: ${({ theme, $online }) =>
		$online ? theme.colors.text[200] : theme.colors.text[300]};

  span {
    color: ${({ theme, $online }) =>
			$online ? theme.colors.success : theme.colors.text[300]};
  }

  ${MOBILE} {
    display: none;
  }
`;

const Body = styled.div`
  display: flex;
  flex: 1;
  min-height: 0;
`;

const Main = styled.main`
  display: flex;
  flex-direction: column;
  flex: 1;
  min-width: 0;
  overflow-y: auto;
`;

const Companion = styled.aside`
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: flex-end;
  gap: 0.35rem;
  width: 300px;
  flex: none;
  padding: 1rem 0 1.25rem 1.25rem;
  border-left: 1px solid ${({ theme }) => theme.colors.border};

  @media (max-width: 1100px) {
    display: none;
  }
`;

const CompanionImage = styled.img`
  width: 100%;
  min-height: 0;
  flex: 0 1 auto;
  object-fit: contain;
  object-position: bottom;
  margin-bottom: 0.75rem;
`;

const Mood = styled.div`
  color: ${({ theme }) => theme.colors.secondary};
  font-style: italic;
  text-align: center;
`;

const Subtle = styled.div`
  color: ${({ theme }) => theme.colors.text[300]};
  font-size: 0.85em;
`;

const Footer = styled.footer`
  display: flex;
  align-items: center;
  gap: 1rem;
  padding: 0.7rem 0 1rem;
  border-top: 1px solid ${({ theme }) => theme.colors.border};
`;

const Hints = styled.div`
  display: flex;
  flex-wrap: wrap;
  gap: 0 0.4rem;
  min-width: 0;
  color: ${({ theme }) => theme.colors.text[200]};
`;

const Hint = styled.span<{ $global?: boolean }>`
  white-space: nowrap;

  /* Alt shortcuts mean nothing on touch screens. */
  ${({ $global }) => $global && "@media (hover: none), (max-width: 640px) { display: none; }"}

  &:not(:last-child)::after {
    content: " ·";
    color: ${({ theme }) => theme.colors.text[300]};
  }
`;

const HintKey = styled.span`
  color: ${({ theme }) => theme.colors.primary};
  font-weight: 700;
`;

const Badge = styled.span`
  margin-left: auto;
  flex: none;
  color: ${({ theme }) => theme.colors.text[300]};

  &::before {
    content: "● ";
    color: ${({ theme }) => theme.colors.success};
  }

  ${MOBILE} {
    display: none;
  }
`;
