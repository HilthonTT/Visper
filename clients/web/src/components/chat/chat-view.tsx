import {
	useCallback,
	useEffect,
	useLayoutEffect,
	useMemo,
	useRef,
	useState,
} from "react";
import styled from "styled-components";
import { useShell } from "@/contexts/shell-context";
import { useRoomChat } from "@/hooks/use-room-chat";
import {
	type ChatMessage,
	memberName,
	searchMessages,
	seenByMessage,
	typingLabel,
	typingNames,
} from "@/lib/chat-state";
import type { ChatSession } from "./join-form";
import { MessageItem } from "./message-item";

type Mode =
	| { kind: "new" }
	| { kind: "reply"; message: ChatMessage }
	| { kind: "edit"; message: ChatMessage };

const MAX_MESSAGE_LENGTH = 700; // leaves room for encryption under the API's 1000

export const ChatView = ({
	session,
	active,
	onLeft,
}: {
	session: ChatSession;
	active: boolean;
	onLeft: () => void;
}) => {
	const chat = useRoomChat(session.room, session.key, active);
	const { state, connection } = chat;
	const { setMood } = useShell();

	const [draft, setDraft] = useState("");
	const [mode, setMode] = useState<Mode>({ kind: "new" });
	const [query, setQuery] = useState("");
	const [searching, setSearching] = useState(false);
	const [showMembers, setShowMembers] = useState(false);
	const [error, setError] = useState("");
	const [busy, setBusy] = useState(false);
	const [now, setNow] = useState(() => Date.now());

	const list = useRef<HTMLDivElement>(null);
	const input = useRef<HTMLTextAreaElement>(null);
	const file = useRef<HTMLInputElement>(null);
	const stick = useRef(true);

	// Re-render the typing line as entries expire.
	useEffect(() => {
		const id = window.setInterval(() => setNow(Date.now()), 1000);
		return () => window.clearInterval(id);
	}, []);

	const visible = useMemo(
		() => searchMessages(state.messages, searching ? query : ""),
		[state.messages, query, searching],
	);
	const byId = useMemo(
		() => new Map(state.messages.map((m) => [m.id, m])),
		[state.messages],
	);
	const seen = useMemo(() => seenByMessage(state), [state]);
	const typing = typingLabel(typingNames(state, now));
	const onlineSet = new Set(state.online);

	// Keep the newest message in view unless the user scrolled up.
	useLayoutEffect(() => {
		const el = list.current;
		if (el && stick.current && visible.length > 0) {
			el.scrollTop = el.scrollHeight;
		}
	}, [visible]);

	const fail = useCallback((e: unknown) => {
		setError(e instanceof Error ? e.message : "Something went wrong.");
	}, []);

	const submit = async () => {
		const text = draft.trim();
		if (!text || busy) {
			return;
		}
		setBusy(true);
		setError("");
		try {
			if (mode.kind === "edit") {
				await chat.edit(mode.message.id, text);
			} else {
				await chat.send(
					text,
					mode.kind === "reply" ? mode.message.id : undefined,
				);
				setMood("cute message ♡");
			}
			setDraft("");
			setMode({ kind: "new" });
			stick.current = true;
		} catch (e) {
			fail(e);
		} finally {
			setBusy(false);
			input.current?.focus();
		}
	};

	const onReply = useCallback((message: ChatMessage) => {
		setMode({ kind: "reply", message });
		input.current?.focus();
	}, []);

	const onEdit = useCallback((message: ChatMessage) => {
		setMode({ kind: "edit", message });
		setDraft(message.content);
		input.current?.focus();
	}, []);

	const onDelete = useCallback(
		(message: ChatMessage) => {
			if (window.confirm("Delete this message for everyone?")) {
				chat.remove(message.id).catch(fail);
			}
		},
		[chat.remove, fail],
	);

	const onReact = useCallback(
		(message: ChatMessage, emoji: string) => {
			chat.react(message, emoji).catch(fail);
		},
		[chat.react, fail],
	);

	const onJumpTo = useCallback((id: string) => {
		setSearching(false);
		requestAnimationFrame(() => {
			const el = document.getElementById(`msg-${id}`);
			el?.scrollIntoView({ block: "center", behavior: "smooth" });
			el?.classList.add("highlight");
			window.setTimeout(() => el?.classList.remove("highlight"), 1500);
		});
	}, []);

	const cancelMode = () => {
		if (mode.kind === "edit") {
			setDraft("");
		}
		setMode({ kind: "new" });
	};

	const onKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
		if (e.key === "Enter" && !e.shiftKey) {
			e.preventDefault();
			submit();
		} else if (e.key === "Escape") {
			cancelMode();
		} else if (e.key === "ArrowUp" && !draft) {
			const last = [...state.messages]
				.reverse()
				.find((m) => m.userId === state.me && !m.failed);
			if (last) {
				e.preventDefault();
				onEdit(last);
			}
		}
	};

	const pickImage = async (f: File | undefined) => {
		if (!f) {
			return;
		}
		setBusy(true);
		setError("");
		try {
			await chat.uploadImage(f);
			stick.current = true;
			setMood("cute pic!");
		} catch (e) {
			fail(e);
		} finally {
			setBusy(false);
			if (file.current) {
				file.current.value = "";
			}
		}
	};

	const leave = async () => {
		if (!window.confirm("Leave this room?")) {
			return;
		}
		await chat.leave().catch(() => {});
		onLeft();
	};

	if (state.closed) {
		return (
			<Closed>
				<p>{state.closed}</p>
				<button type="button" onClick={onLeft}>
					back ↵
				</button>
			</Closed>
		);
	}

	return (
		<Layout>
			<Bar>
				<RoomPill title="join code">#{state.joinCode}</RoomPill>
				<Conn $state={connection}>
					{connection === "open"
						? `● ${state.online.length} online`
						: connection === "reconnecting"
							? "○ reconnecting…"
							: "○ connecting…"}
				</Conn>
				<BarSpacer />
				{searching ? (
					<Search
						autoFocus
						aria-label="search messages"
						placeholder="search…"
						value={query}
						onChange={(e) => setQuery(e.target.value)}
						onKeyDown={(e) => {
							if (e.key === "Escape") {
								setQuery("");
								setSearching(false);
							}
						}}
					/>
				) : null}
				<BarButton
					type="button"
					aria-pressed={searching}
					onClick={() => {
						setSearching((s) => !s);
						setQuery("");
					}}
				>
					{searching ? "close search" : "search"}
				</BarButton>
				<BarButton
					type="button"
					className="members-toggle"
					aria-pressed={showMembers}
					onClick={() => setShowMembers((s) => !s)}
				>
					members
				</BarButton>
				<BarButton type="button" $danger onClick={leave}>
					leave
				</BarButton>
			</Bar>

			<Body>
				<Messages
					ref={list}
					role="log"
					aria-live="polite"
					onScroll={(e) => {
						const el = e.currentTarget;
						stick.current =
							el.scrollHeight - el.scrollTop - el.clientHeight < 40;
					}}
				>
					{searching && query.trim() && (
						<Hint>
							{visible.length} result{visible.length === 1 ? "" : "s"} in the{" "}
							{state.messages.length} loaded messages
						</Hint>
					)}
					{state.messages.length === 0 && (
						<Hint>
							No messages yet. Messages are encrypted before they leave this
							page.
						</Hint>
					)}
					{visible.map((m) => (
						<MessageItem
							key={m.id}
							message={m}
							me={state.me}
							roomKey={session.key}
							replyTo={
								m.replyToId ? (byId.get(m.replyToId) ?? null) : undefined
							}
							seenBy={seen.get(m.id)}
							onReply={onReply}
							onEdit={onEdit}
							onDelete={onDelete}
							onReact={onReact}
							onJumpTo={onJumpTo}
						/>
					))}
				</Messages>

				<Members $open={showMembers} aria-label="Members">
					<MembersTitle>members</MembersTitle>
					{[...state.members]
						.sort(
							(a, b) =>
								Number(onlineSet.has(b.id)) - Number(onlineSet.has(a.id)) ||
								a.username.localeCompare(b.username),
						)
						.map((m) => (
							<MemberRow key={m.id} $online={onlineSet.has(m.id)}>
								<span>{onlineSet.has(m.id) ? "●" : "○"}</span>{" "}
								{m.id === state.me ? `${m.username} (you)` : m.username}
								{m.id === state.ownerId && <Muted> owner</Muted>}
							</MemberRow>
						))}
				</Members>
			</Body>

			<Typing aria-live="polite">{typing}</Typing>

			{mode.kind !== "new" && (
				<ModeBar>
					{mode.kind === "reply" ? (
						<>
							replying to{" "}
							<strong>{memberName(state, mode.message.userId)}</strong>
						</>
					) : (
						"editing your message"
					)}
					<button type="button" onClick={cancelMode}>
						esc cancel
					</button>
				</ModeBar>
			)}
			{error && <ErrorLine role="alert">✕ {error}</ErrorLine>}

			<Composer
				onSubmit={(e) => {
					e.preventDefault();
					submit();
				}}
			>
				<Prompt>›</Prompt>
				<Textarea
					ref={input}
					rows={1}
					aria-label="message"
					placeholder="Write a message…"
					maxLength={MAX_MESSAGE_LENGTH}
					value={draft}
					onChange={(e) => {
						setDraft(e.target.value);
						if (e.target.value) {
							chat.notifyTyping();
						}
					}}
					onKeyDown={onKeyDown}
				/>
				<input
					ref={file}
					type="file"
					accept="image/png,image/jpeg,image/gif,image/webp,image/bmp"
					hidden
					onChange={(e) => pickImage(e.target.files?.[0])}
				/>
				<ComposerButton
					type="button"
					disabled={busy}
					title="send an encrypted image"
					onClick={() => file.current?.click()}
				>
					img
				</ComposerButton>
				<ComposerButton type="submit" $primary disabled={busy || !draft.trim()}>
					{mode.kind === "edit" ? "save" : "send"}
				</ComposerButton>
			</Composer>
		</Layout>
	);
};

const NARROW = "@media (max-width: 800px)";

const Layout = styled.div`
  display: flex;
  flex-direction: column;
  flex: 1;
  min-height: 0;
  padding-top: 0.6rem;
`;

const Bar = styled.div`
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 0.5rem 0.8rem;
  padding-bottom: 0.5rem;
  border-bottom: 1px solid ${({ theme }) => theme.colors.border};
`;

const RoomPill = styled.span`
  padding: 0 0.5rem;
  border-radius: 3px;
  background: ${({ theme }) => theme.colors.surfaceAlt};
  color: ${({ theme }) => theme.colors.primary};
  font-weight: 700;
`;

const Conn = styled.span<{ $state: string }>`
  color: ${({ theme, $state }) =>
		$state === "open" ? theme.colors.success : theme.colors.warning};
`;

const BarSpacer = styled.span`
  flex: 1;
`;

const Search = styled.input`
  width: min(14rem, 100%);
  padding: 0 0.5rem;
  border: 1px solid ${({ theme }) => theme.colors.primary};
  border-radius: 3px;
`;

const BarButton = styled.button<{ $danger?: boolean }>`
  color: ${({ theme, $danger }) =>
		$danger ? theme.colors.danger : theme.colors.text[200]};

  &:hover,
  &[aria-pressed="true"] {
    color: ${({ theme, $danger }) =>
			$danger ? theme.colors.danger : theme.colors.primary};
  }

  &.members-toggle {
    display: none;

    ${NARROW} {
      display: inline;
    }
  }
`;

const Body = styled.div`
  display: flex;
  flex: 1;
  min-height: 0;
`;

const Messages = styled.div`
  flex: 1;
  min-width: 0;
  overflow-y: auto;
  padding: 0.5rem 0;
`;

const Hint = styled.div`
  padding: 0.5rem 0.6rem;
  color: ${({ theme }) => theme.colors.text[300]};
`;

const Members = styled.aside<{ $open: boolean }>`
  width: 13rem;
  flex: none;
  padding: 0.5rem 0 0.5rem 0.9rem;
  border-left: 1px solid ${({ theme }) => theme.colors.border};
  overflow-y: auto;

  ${NARROW} {
    display: ${({ $open }) => ($open ? "block" : "none")};
    width: 11rem;
  }
`;

const MembersTitle = styled.div`
  margin-bottom: 0.3rem;
  color: ${({ theme }) => theme.colors.text[300]};
`;

const MemberRow = styled.div<{ $online: boolean }>`
  overflow: hidden;
  color: ${({ theme, $online }) =>
		$online ? theme.colors.text[100] : theme.colors.text[300]};
  text-overflow: ellipsis;
  white-space: nowrap;

  span {
    color: ${({ theme, $online }) =>
			$online ? theme.colors.success : theme.colors.text[300]};
  }
`;

const Muted = styled.span`
  color: ${({ theme }) => theme.colors.text[300]} !important;
`;

const Typing = styled.div`
  min-height: 1.4em;
  color: ${({ theme }) => theme.colors.secondary};
  font-style: italic;
  font-size: 0.9em;
`;

const ModeBar = styled.div`
  display: flex;
  gap: 1rem;
  color: ${({ theme }) => theme.colors.text[200]};

  strong {
    color: ${({ theme }) => theme.colors.text[100]};
  }

  button {
    margin-left: auto;
    color: ${({ theme }) => theme.colors.text[300]};
  }
`;

const ErrorLine = styled.div`
  color: ${({ theme }) => theme.colors.danger};
`;

const Composer = styled.form`
  display: flex;
  align-items: flex-end;
  gap: 0.6rem;
  margin: 0.3rem 0 0.6rem;
  padding: 0.45rem 0.8rem;
  border: 1px solid ${({ theme }) => theme.colors.primary};
  border-radius: 8px;
`;

const Prompt = styled.span`
  color: ${({ theme }) => theme.colors.primary};
  font-weight: 700;
`;

const Textarea = styled.textarea`
  flex: 1;
  min-width: 0;
  max-height: 8rem;
  padding: 0;
  border: none;
  background: transparent;
  color: inherit;
  font: inherit;
  resize: none;
  field-sizing: content;
`;

const ComposerButton = styled.button<{ $primary?: boolean }>`
  flex: none;
  padding: 0 0.6rem;
  border-radius: 3px;
  background: ${({ theme, $primary }) =>
		$primary ? theme.colors.primary : theme.colors.surfaceAlt};
  color: ${({ theme, $primary }) =>
		$primary ? theme.colors.onPrimary : theme.colors.text[200]};
  font-weight: 700;

  &:disabled {
    opacity: 0.5;
  }
`;

const Closed = styled.div`
  margin: auto;
  text-align: center;
  color: ${({ theme }) => theme.colors.text[200]};

  button {
    margin-top: 0.75rem;
    color: ${({ theme }) => theme.colors.primary};
  }
`;
