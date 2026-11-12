import type { ApiMessage, ApiRoom } from "./chat-api";
import { type RoomKey, readField } from "./room-crypto";

/*
 * The browser chat's state, as a pure reducer over room events. Everything
 * arriving from the server is decrypted before it gets here (see
 * parseRoomEvent and fromApiMessage), so the state only holds plaintext.
 */

export type Reaction = { userId: string; emoji: string };

export type ChatMessage = {
	id: string;
	userId: string;
	username: string;
	/** Decrypted content, or a placeholder if decryption failed. */
	content: string;
	failed: boolean;
	/** Milliseconds since the epoch. */
	createdAt: number;
	replyToId?: string;
	reactions: Reaction[];
	edited?: boolean;
};

export type Member = { id: string; username: string };

export type ChatState = {
	roomId: string;
	joinCode: string;
	ownerId: string;
	me: string;
	expiresAt: number;
	messages: ChatMessage[];
	members: Member[];
	/** IDs of the members connected right now. */
	online: string[];
	/** Who is typing, until when (ms). */
	typing: Record<string, { username: string; until: number }>;
	/** Last message each member has read. */
	reads: Record<string, string>;
	/** Set once the room is gone for us. */
	closed?: string;
};

/** How long a typing event counts; the server relays one per ~2s. */
export const TYPING_TTL_MS = 5000;

export function initialChatState(room: ApiRoom): ChatState {
	const members = [...(room.members ?? [])];
	if (!members.some((m) => m.id === room.current_user.id)) {
		members.push(room.current_user);
	}
	return {
		roomId: room.id,
		joinCode: room.join_code,
		ownerId: room.owner.id,
		me: room.current_user.id,
		expiresAt: Date.parse(room.expires_at) || 0,
		messages: [],
		members: members.map((m) => ({ id: m.id, username: m.username })),
		online: [],
		typing: {},
		reads: {},
	};
}

export type ChatAction =
	| { type: "messages"; messages: ChatMessage[] }
	| { type: "updated"; id: string; content: string; failed: boolean }
	| { type: "deleted"; id: string }
	| { type: "reactions"; id: string; reactions: Reaction[] }
	| { type: "joined"; member: Member }
	| { type: "left"; userId: string }
	| { type: "kicked"; userId: string; reason: string }
	| { type: "presence"; online: string[] }
	| { type: "typing"; userId: string; username: string; now: number }
	| { type: "stopTyping"; userId: string }
	| { type: "expireTyping"; now: number }
	| { type: "read"; userId: string; messageId: string }
	| { type: "roomUpdated"; joinCode: string }
	| { type: "closed"; reason: string };

export function chatReducer(state: ChatState, action: ChatAction): ChatState {
	switch (action.type) {
		case "messages": {
			if (action.messages.length === 0) {
				return state;
			}
			const byId = new Map(state.messages.map((m) => [m.id, m]));
			let typing = state.typing;
			for (const m of action.messages) {
				byId.set(m.id, m);
				// A message from someone ends their typing indicator.
				if (typing[m.userId]) {
					typing = { ...typing };
					delete typing[m.userId];
				}
			}
			const messages = [...byId.values()].sort(
				(a, b) => a.createdAt - b.createdAt || a.id.localeCompare(b.id),
			);
			const members = addMembers(
				state.members,
				action.messages.map((m) => ({ id: m.userId, username: m.username })),
			);
			return { ...state, messages, members, typing };
		}
		case "updated":
			return mapMessage(state, action.id, (m) => ({
				...m,
				content: action.content,
				failed: action.failed,
				edited: true,
			}));
		case "deleted":
			return {
				...state,
				messages: state.messages.filter((m) => m.id !== action.id),
			};
		case "reactions":
			return mapMessage(state, action.id, (m) => ({
				...m,
				reactions: action.reactions,
			}));
		case "joined":
			return { ...state, members: addMembers(state.members, [action.member]) };
		case "left":
		case "kicked": {
			const next = {
				...state,
				members: state.members.filter((m) => m.id !== action.userId),
				online: state.online.filter((id) => id !== action.userId),
			};
			if (action.type === "kicked" && action.userId === state.me) {
				next.closed = action.reason
					? `You were removed from the room: ${action.reason}`
					: "You were removed from the room.";
			}
			return next;
		}
		case "presence":
			return { ...state, online: [...action.online] };
		case "typing":
			if (action.userId === state.me) {
				return state;
			}
			return {
				...state,
				typing: {
					...state.typing,
					[action.userId]: {
						username: action.username,
						until: action.now + TYPING_TTL_MS,
					},
				},
			};
		case "stopTyping": {
			if (!state.typing[action.userId]) {
				return state;
			}
			const typing = { ...state.typing };
			delete typing[action.userId];
			return { ...state, typing };
		}
		case "expireTyping": {
			const entries = Object.entries(state.typing);
			const live = entries.filter(([, t]) => t.until > action.now);
			if (live.length === entries.length) {
				return state;
			}
			return { ...state, typing: Object.fromEntries(live) };
		}
		case "read":
			if (state.reads[action.userId] === action.messageId) {
				return state;
			}
			return {
				...state,
				reads: { ...state.reads, [action.userId]: action.messageId },
			};
		case "roomUpdated":
			return { ...state, joinCode: action.joinCode };
		case "closed":
			return { ...state, closed: action.reason };
	}
}

function mapMessage(
	state: ChatState,
	id: string,
	fn: (m: ChatMessage) => ChatMessage,
): ChatState {
	const i = state.messages.findIndex((m) => m.id === id);
	if (i < 0) {
		return state;
	}
	const messages = [...state.messages];
	messages[i] = fn(messages[i]);
	return { ...state, messages };
}

function addMembers(members: Member[], add: Member[]): Member[] {
	let out = members;
	for (const m of add) {
		if (!m.id) {
			continue;
		}
		const i = out.findIndex((x) => x.id === m.id);
		if (i < 0) {
			out = [...out, m];
		} else if (m.username && out[i].username !== m.username) {
			out = out.map((x, j) => (j === i ? { ...x, username: m.username } : x));
		}
	}
	return out;
}

// ---------------------------------------------------------------------------
// Decoding server data

type RawReaction = {
	userId?: unknown;
	user_id?: unknown;
	emoji?: unknown;
	encrypted?: unknown;
};

const str = (v: unknown): string => (typeof v === "string" ? v : "");

function decodeReactions(raw: unknown, key: RoomKey): Reaction[] {
	if (!Array.isArray(raw)) {
		return [];
	}
	const out: Reaction[] = [];
	for (const r of raw as RawReaction[]) {
		const userId = str(r?.userId) || str(r?.user_id);
		if (!userId) {
			continue;
		}
		const { text } = readField(str(r.emoji), r.encrypted === true, key);
		out.push({ userId, emoji: text });
	}
	return out;
}

/** Decrypts a message from the REST API. */
export function fromApiMessage(m: ApiMessage, key: RoomKey): ChatMessage {
	const { text, failed } = readField(m.content, m.encrypted, key);
	return {
		id: m.id,
		userId: m.user_id,
		username: m.username,
		content: text,
		failed,
		createdAt: Date.parse(m.created_at) || 0,
		replyToId: m.reply_to_id || undefined,
		reactions: decodeReactions(m.reactions, key),
	};
}

/**
 * Turns one room WebSocket event into a reducer action, decrypting what it
 * carries. Unknown or malformed events return null.
 */
export function parseRoomEvent(
	raw: unknown,
	key: RoomKey,
	now = Date.now(),
): ChatAction | null {
	if (typeof raw !== "object" || raw === null) {
		return null;
	}
	const { type, data } = raw as { type?: unknown; data?: unknown };
	const d = (typeof data === "object" && data !== null ? data : {}) as Record<
		string,
		unknown
	>;

	switch (type) {
		case "message.received": {
			const id = str(d.id);
			const userId = str(d.userId);
			if (!id || !userId) {
				return null;
			}
			const { text, failed } = readField(
				str(d.content),
				d.encrypted === true,
				key,
			);
			return {
				type: "messages",
				messages: [
					{
						id,
						userId,
						username: str(d.username),
						content: text,
						failed,
						createdAt: Date.parse(str(d.timestamp)) || now,
						replyToId: str(d.replyToId) || undefined,
						reactions: decodeReactions(d.reactions, key),
					},
				],
			};
		}
		case "message.updated": {
			const id = str(d.id);
			if (!id) {
				return null;
			}
			const { text, failed } = readField(
				str(d.content),
				d.encrypted === true,
				key,
			);
			return { type: "updated", id, content: text, failed };
		}
		case "message.deleted":
			return str(d.id) ? { type: "deleted", id: str(d.id) } : null;
		case "message.reactions":
			return str(d.id)
				? {
						type: "reactions",
						id: str(d.id),
						reactions: decodeReactions(d.reactions, key),
					}
				: null;
		case "member.joined":
			return str(d.userId)
				? {
						type: "joined",
						member: { id: str(d.userId), username: str(d.username) },
					}
				: null;
		case "member.left":
			return str(d.userId) ? { type: "left", userId: str(d.userId) } : null;
		case "error.kicked":
			return str(d.userId)
				? { type: "kicked", userId: str(d.userId), reason: str(d.reason) }
				: null;
		case "presence.update":
			return Array.isArray(d.online)
				? {
						type: "presence",
						online: d.online.filter((x) => typeof x === "string"),
					}
				: null;
		case "member.typing":
			return str(d.userId)
				? {
						type: "typing",
						userId: str(d.userId),
						username: str(d.username),
						now,
					}
				: null;
		case "read.update":
			return str(d.userId) && str(d.messageId)
				? { type: "read", userId: str(d.userId), messageId: str(d.messageId) }
				: null;
		case "room.updated":
			return str(d.joinCode)
				? { type: "roomUpdated", joinCode: str(d.joinCode) }
				: null;
		case "room.deleted":
			return { type: "closed", reason: "The owner deleted this room." };
	}
	return null;
}

// ---------------------------------------------------------------------------
// Views over the state

export type ReactionGroup = { emoji: string; count: number; mine: boolean };

/** Groups a message's reactions by emoji, most used first. */
export function groupReactions(
	reactions: Reaction[],
	me: string,
): ReactionGroup[] {
	const groups = new Map<string, ReactionGroup>();
	for (const r of reactions) {
		const g = groups.get(r.emoji) ?? { emoji: r.emoji, count: 0, mine: false };
		g.count++;
		g.mine ||= r.userId === me;
		groups.set(r.emoji, g);
	}
	return [...groups.values()].sort((a, b) => b.count - a.count);
}

/** The emoji `me` reacted with to a message, if any. */
export function myReaction(reactions: Reaction[], me: string): string | null {
	return reactions.find((r) => r.userId === me)?.emoji ?? null;
}

/**
 * Who has read up to each message: members other than `me` and the author,
 * placed at their last-read message.
 */
export function seenByMessage(state: ChatState): Map<string, string[]> {
	const out = new Map<string, string[]>();
	const ids = new Set(state.messages.map((m) => m.id));
	for (const [userId, messageId] of Object.entries(state.reads)) {
		if (userId === state.me || !ids.has(messageId)) {
			continue;
		}
		const name = memberName(state, userId);
		out.set(messageId, [...(out.get(messageId) ?? []), name]);
	}
	return out;
}

export function memberName(state: ChatState, userId: string): string {
	return (
		state.members.find((m) => m.id === userId)?.username ||
		state.messages.find((m) => m.userId === userId)?.username ||
		"someone"
	);
}

/** Names of the members typing right now, excluding `me`. */
export function typingNames(state: ChatState, now: number): string[] {
	return Object.entries(state.typing)
		.filter(([id, t]) => id !== state.me && t.until > now)
		.map(([, t]) => t.username || "someone");
}

export function typingLabel(names: string[]): string {
	switch (names.length) {
		case 0:
			return "";
		case 1:
			return `${names[0]} is typing…`;
		case 2:
			return `${names[0]} and ${names[1]} are typing…`;
		default:
			return "several people are typing…";
	}
}

/** Messages whose decrypted text or author matches the query. */
export function searchMessages(
	messages: ChatMessage[],
	query: string,
): ChatMessage[] {
	const q = query.trim().toLowerCase();
	if (!q) {
		return messages;
	}
	return messages.filter(
		(m) =>
			!m.failed &&
			(m.content.toLowerCase().includes(q) ||
				m.username.toLowerCase().includes(q)),
	);
}
