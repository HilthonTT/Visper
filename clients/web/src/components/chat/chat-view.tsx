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
import { isEncryptedImageUrl, isPlainImageUrl } from "@/lib/room-crypto";
import { FilesPanel } from "./files-panel";
import { InvitePanel } from "./invite-panel";
import type { ChatSession } from "./join-form";
import { KeysPanel } from "./keys-panel";
import { MessageItem } from "./message-item";

type Mode =
	| { kind: "new" }
	| { kind: "reply"; message: ChatMessage }
	| { kind: "edit"; message: ChatMessage };

// Leaves room for the signed, encrypted envelope under the API's 4000.
const MAX_MESSAGE_LENGTH = 2500;

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
	// A room you just created is empty: start with what to share.
	const [showInvite, setShowInvite] = useState(Boolean(session.created));
	// The member whose key fingerprint is shown: null for only ours,
	// undefined when the keys panel is closed.
	const [keysFor, setKeysFor] = useState<string | null | undefined>();
	const [showFiles, setShowFiles] = useState(false);
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
				.find(
					(m) =>
						m.userId === state.me &&
						!m.failed &&
						!isEncryptedImageUrl(m.content) &&
						!isPlainImageUrl(m.content),
				);
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
		try {
			await chat.leave();
		} catch (e) {
			fail(e);
			return;
		}
		onLeft();
	};

	const isOwner = state.ownerId === state.me;

	const deleteRoom = async () => {
		if (
			!window.confirm(
				"Delete this room for everyone? Its messages and files are gone for good.",
			)
		) {
			return;
		}
		try {
			await chat.deleteRoom();
		} catch (e) {
			fail(e);
			return;
		}
		onLeft();
	};

	const newJoinCode = async () => {
		if (
			!window.confirm(
				"Make a new join code? The current code stops letting people join. Members stay, and the room key stays the same.",
			)
		) {
			return;
		}
		setError("");
		try {
			await chat.newJoinCode();
			setShowInvite(true);
		} catch (e) {
			fail(e);
		}
	};

	const kick = async (member: { id: string; username: string }) => {
		if (
			!window.confirm(
				`Remove ${member.username} from the room? The room key is rotated so they can't read what comes next.`,
			)
		) {
			return;
		}
		setError("");
		try {
			await chat.kick(member.id);
			if (keysFor === member.id) {
				setKeysFor(undefined);
			}
		} catch (e) {
			fail(e);
		}
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
					aria-pressed={showInvite}
					onClick={() => setShowInvite((s) => !s)}
				>
					invite
				</BarButton>
				<BarButton
					type="button"
					aria-pressed={keysFor !== undefined}
					onClick={() =>
						setKeysFor((k) => (k === undefined ? null : undefined))
					}
				>
					keys
				</BarButton>
				<BarButton
					type="button"
					aria-pressed={showFiles}
					onClick={() => setShowFiles((s) => !s)}
				>
					files
				</BarButton>
				<BarButton
					type="button"
					className="members-toggle"
					aria-pressed={showMembers}
					onClick={() => setShowMembers((s) => !s)}
				>
					members
				</BarButton>
				{isOwner && (
					<BarButton
						type="button"
						title="replace the join code"
						onClick={newJoinCode}
					>
						new code
					</BarButton>
				)}
				<BarButton type="button" $danger onClick={leave}>
					leave
				</BarButton>
				{isOwner && (
					<BarButton
						type="button"
						$danger
						title="delete the room for everyone"
						onClick={deleteRoom}
					>
						delete
					</BarButton>
				)}
			</Bar>
			{showFiles && (
				<FilesPanel
					state={state}
					list={chat.listFiles}
					remove={chat.deleteFile}
					onJumpTo={onJumpTo}
					onClose={() => setShowFiles(false)}
				/>
			)}
			{showInvite && (
				<InvitePanel
					joinCode={state.joinCode}
					joinUrl={session.room.qr_code_url}
					roomKey={chat.inviteKey}
					onClose={() => setShowInvite(false)}
				/>
			)}
			{keysFor !== undefined && (
				<KeysPanel
					member={
						keysFor === null
							? null
							: (state.members.find((m) => m.id === keysFor) ?? {
									id: keysFor,
									username: memberName(state, keysFor),
								})
					}
					myFingerprint={chat.crypto.myFingerprint()}
					fingerprint={keysFor ? chat.crypto.fingerprint(keysFor) : ""}
					status={keysFor ? chat.crypto.keyStatus(keysFor) : "unverified"}
					onVerify={() => {
						if (!keysFor) {
							return;
						}
						try {
							chat.verifyMember(keysFor, chat.crypto.fingerprint(keysFor));
						} catch (e) {
							fail(e);
						}
					}}
					onUnverify={() => keysFor && chat.unverifyMember(keysFor)}
					onClose={() => setKeysFor(undefined)}
				/>
			)}

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
							crypto={chat.crypto}
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
						.map((m) => {
							const status =
								m.id === state.me ? null : chat.crypto.keyStatus(m.id);
							return (
								<MemberRow key={m.id} $online={onlineSet.has(m.id)}>
									<span>{onlineSet.has(m.id) ? "●" : "○"}</span>
									<MemberButton
										type="button"
										title={
											m.id === state.me
												? "show your key"
												: `compare keys with ${m.username}`
										}
										onClick={() => setKeysFor(m.id === state.me ? null : m.id)}
									>
										{m.username}
									</MemberButton>
									{m.id === state.me && <Muted>you</Muted>}
									{status === "verified" && (
										<Trust $tone="success" title="key verified">
											✓
										</Trust>
									)}
									{status === "changed" && (
										<Trust $tone="warning" title="key changed since verified">
											!
										</Trust>
									)}
									{m.id === state.ownerId && <Muted>owner</Muted>}
									{isOwner && m.id !== state.me && (
										<KickButton
											type="button"
											className="kick"
											title={`remove ${m.username} from the room`}
											onClick={() => kick(m)}
										>
											kick
										</KickButton>
									)}
								</MemberRow>
							);
						})}
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
			{chat.notice && (
				<NoticeLine role="status">
					🔑 {chat.notice}{" "}
					<BarButton type="button" onClick={chat.clearNotice}>
						dismiss
					</BarButton>
				</NoticeLine>
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
  display: flex;
  flex-direction: column;
  gap: 0.15rem;
  width: 15rem;
  flex: none;
  padding: 0.5rem 0.25rem 0.5rem 0.9rem;
  border-left: 1px solid ${({ theme }) => theme.colors.border};
  overflow-y: auto;

  ${NARROW} {
    display: ${({ $open }) => ($open ? "flex" : "none")};
    width: 11rem;
  }
`;

const MembersTitle = styled.div`
  margin-bottom: 0.3rem;
  color: ${({ theme }) => theme.colors.text[300]};
`;

// The name gives way to the badges: a long name truncates, but ✓, ! and
// "owner" always stay in view.
const MemberRow = styled.div<{ $online: boolean }>`
  display: flex;
  align-items: baseline;
  gap: 1ch;
  color: ${({ theme, $online }) =>
		$online ? theme.colors.text[100] : theme.colors.text[300]};
  white-space: nowrap;

  & > span {
    flex: none;
  }

  &:hover > button.kick {
    opacity: 1;
  }

  span {
    color: ${({ theme, $online }) =>
			$online ? theme.colors.success : theme.colors.text[300]};
  }
`;

const Muted = styled.span`
  color: ${({ theme }) => theme.colors.text[300]} !important;
`;

const MemberButton = styled.button`
  min-width: 0;
  overflow: hidden;
  color: inherit;
  font: inherit;
  text-align: left;
  text-overflow: ellipsis;

  &:hover {
    color: ${({ theme }) => theme.colors.primary};
  }
`;

// Shown only on hover or keyboard focus, so the list stays calm.
const KickButton = styled.button`
  flex: none;
  margin-left: auto;
  padding-right: 0.4rem;
  color: ${({ theme }) => theme.colors.danger};
  opacity: 0;

  &:focus-visible {
    opacity: 1;
  }

  @media (hover: none) {
    opacity: 1;
  }
`;

const Trust = styled.span<{ $tone: "success" | "warning" }>`
  color: ${({ theme, $tone }) => theme.colors[$tone]} !important;
  font-weight: 700;
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

const NoticeLine = styled.div`
  color: ${({ theme }) => theme.colors.text[300]};
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
