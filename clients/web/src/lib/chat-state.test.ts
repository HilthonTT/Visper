import { describe, expect, it } from "vitest";
import type { ApiRoom } from "./chat-api";
import {
	type ChatAction,
	type ChatMessage,
	type ChatState,
	chatReducer,
	fromApiMessage,
	groupReactions,
	initialChatState,
	parseRoomEvent,
	searchMessages,
	seenByMessage,
	TYPING_TTL_MS,
	typingLabel,
	typingNames,
} from "./chat-state";
import { encryptText, type RoomKey } from "./room-crypto";

const key = new Uint8Array(32).map((_, i) => i) as RoomKey;

const room: ApiRoom = {
	id: "r1",
	join_code: "ABC123",
	owner: { id: "ann", username: "Ann" },
	created_at: "2026-01-01T00:00:00Z",
	expires_at: "2026-01-02T00:00:00Z",
	members: [{ id: "ann", username: "Ann" }],
	current_user: { id: "me", username: "Me" },
	qr_code_url: "",
};

const msg = (
	id: string,
	userId: string,
	content: string,
	at: number,
): ChatMessage => ({
	id,
	userId,
	username: userId,
	content,
	failed: false,
	createdAt: at,
	reactions: [],
});

const apply = (state: ChatState, ...actions: (ChatAction | null)[]) =>
	actions.reduce((s, a) => (a ? chatReducer(s, a) : s), state);

describe("initialChatState", () => {
	it("includes the current user among the members", () => {
		const s = initialChatState(room);
		expect(s.me).toBe("me");
		expect(s.members.map((m) => m.id)).toEqual(["ann", "me"]);
	});
});

describe("parseRoomEvent", () => {
	it("decrypts received messages and their reactions", () => {
		const action = parseRoomEvent(
			{
				type: "message.received",
				data: {
					id: "m1",
					userId: "ann",
					username: "Ann",
					content: encryptText("secret", key),
					encrypted: true,
					timestamp: "2026-01-01T10:00:00Z",
					replyToId: "m0",
					reactions: [
						{ userId: "me", emoji: encryptText("👍", key), encrypted: true },
					],
				},
			},
			key,
		);
		expect(action).toMatchObject({
			type: "messages",
			messages: [
				{
					id: "m1",
					content: "secret",
					failed: false,
					replyToId: "m0",
					reactions: [{ userId: "me", emoji: "👍" }],
				},
			],
		});
	});

	it("marks messages it can't decrypt", () => {
		const other = new Uint8Array(32).fill(1) as RoomKey;
		const action = parseRoomEvent(
			{
				type: "message.received",
				data: {
					id: "m1",
					userId: "ann",
					content: encryptText("x", other),
					encrypted: true,
				},
			},
			key,
		);
		expect(action).toMatchObject({
			messages: [{ failed: true, content: "[Decryption failed]" }],
		});
	});

	it.each([
		[
			{ type: "presence.update", data: { online: ["ann", 3] } },
			{ type: "presence", online: ["ann"] },
		],
		[
			{ type: "read.update", data: { userId: "ann", messageId: "m1" } },
			{ type: "read", userId: "ann", messageId: "m1" },
		],
		[
			{ type: "member.typing", data: { userId: "ann", username: "Ann" } },
			{ type: "typing", userId: "ann", now: 5 },
		],
		[
			{ type: "message.deleted", data: { id: "m1" } },
			{ type: "deleted", id: "m1" },
		],
		[{ type: "room.deleted", data: {} }, { type: "closed" }],
	])("parses %j", (raw, want) => {
		expect(parseRoomEvent(raw, key, 5)).toMatchObject(want);
	});

	it.each([
		null,
		"x",
		{},
		{ type: "message.received", data: {} },
		{ type: "nope", data: {} },
	])("ignores %j", (raw) => {
		expect(parseRoomEvent(raw, key)).toBeNull();
	});
});

describe("chatReducer", () => {
	it("merges history and live messages by id, in time order", () => {
		const s = apply(
			initialChatState(room),
			{
				type: "messages",
				messages: [msg("b", "ann", "2", 2), msg("a", "ann", "1", 1)],
			},
			{
				type: "messages",
				messages: [msg("b", "ann", "2", 2), msg("c", "bo", "3", 3)],
			},
		);
		expect(s.messages.map((m) => m.id)).toEqual(["a", "b", "c"]);
		expect(s.members.some((m) => m.id === "bo")).toBe(true);
	});

	it("updates, reacts to and deletes messages", () => {
		const s = apply(
			initialChatState(room),
			{ type: "messages", messages: [msg("a", "ann", "hi", 1)] },
			{ type: "updated", id: "a", content: "hello", failed: false },
			{
				type: "reactions",
				id: "a",
				reactions: [{ userId: "me", emoji: "🎉" }],
			},
		);
		expect(s.messages[0]).toMatchObject({
			content: "hello",
			edited: true,
			reactions: [{ emoji: "🎉" }],
		});
		expect(apply(s, { type: "deleted", id: "a" }).messages).toEqual([]);
	});

	it("tracks typing until it expires or the person sends", () => {
		let s = apply(
			initialChatState(room),
			{ type: "typing", userId: "ann", username: "Ann", now: 0 },
			{ type: "typing", userId: "me", username: "Me", now: 0 },
		);
		expect(typingNames(s, 1)).toEqual(["Ann"]);
		expect(
			apply(s, { type: "expireTyping", now: TYPING_TTL_MS + 1 }).typing,
		).toEqual({});

		s = apply(s, { type: "messages", messages: [msg("a", "ann", "hi", 1)] });
		expect(typingNames(s, 1)).toEqual([]);
	});

	it("closes the room when we are kicked, not when others are", () => {
		const base = apply(initialChatState(room), {
			type: "presence",
			online: ["ann", "me"],
		});
		const s = apply(base, { type: "kicked", userId: "ann", reason: "" });
		expect(s.closed).toBeUndefined();
		expect(s.online).toEqual(["me"]);
		expect(
			apply(base, { type: "kicked", userId: "me", reason: "spam" }).closed,
		).toContain("spam");
	});
});

describe("views", () => {
	it("groups reactions with mine flagged", () => {
		expect(
			groupReactions(
				[
					{ userId: "a", emoji: "👍" },
					{ userId: "me", emoji: "👍" },
					{ userId: "b", emoji: "❤️" },
				],
				"me",
			),
		).toEqual([
			{ emoji: "👍", count: 2, mine: true },
			{ emoji: "❤️", count: 1, mine: false },
		]);
	});

	it("places read markers of others on their last-read message", () => {
		const s = apply(
			initialChatState(room),
			{ type: "messages", messages: [msg("a", "me", "hi", 1)] },
			{ type: "read", userId: "ann", messageId: "a" },
			{ type: "read", userId: "me", messageId: "a" },
			{ type: "read", userId: "bo", messageId: "gone" },
		);
		expect(seenByMessage(s).get("a")).toEqual(["Ann"]);
		expect(seenByMessage(s).has("gone")).toBe(false);
	});

	it("searches decrypted text and authors", () => {
		const list = [
			msg("a", "ann", "Hello world", 1),
			msg("b", "bo", "bye", 2),
			{ ...msg("c", "cy", "[Decryption failed]", 3), failed: true },
		];
		expect(searchMessages(list, "WORLD").map((m) => m.id)).toEqual(["a"]);
		expect(searchMessages(list, "bo").map((m) => m.id)).toEqual(["b"]);
		expect(searchMessages(list, "decryption")).toEqual([]);
		expect(searchMessages(list, "  ")).toHaveLength(3);
	});

	it("labels typing", () => {
		expect(typingLabel([])).toBe("");
		expect(typingLabel(["Ann"])).toBe("Ann is typing…");
		expect(typingLabel(["Ann", "Bo"])).toBe("Ann and Bo are typing…");
		expect(typingLabel(["a", "b", "c"])).toBe("several people are typing…");
	});

	it("decrypts REST messages", () => {
		const m = fromApiMessage(
			{
				id: "m1",
				room_id: "r1",
				user_id: "ann",
				username: "Ann",
				content: encryptText("from rest", key),
				encrypted: true,
				created_at: "2026-01-01T00:00:00Z",
				reactions: [
					{ user_id: "bo", emoji: encryptText("😂", key), encrypted: true },
				],
			},
			key,
		);
		expect(m).toMatchObject({
			content: "from rest",
			reactions: [{ userId: "bo", emoji: "😂" }],
		});
	});
});

describe("chatReducer history merges", () => {
	it("does not resurrect a deleted message from a late snapshot", () => {
		const s = apply(
			initialChatState(room),
			{ type: "messages", messages: [msg("m1", "ann", "hi", 1)] },
			{ type: "deleted", id: "m1" },
			{ type: "messages", messages: [msg("m1", "ann", "hi", 1)] },
		);
		expect(s.messages).toEqual([]);
	});

	it("keeps the edited marker across a history replay", () => {
		const s = apply(
			initialChatState(room),
			{ type: "messages", messages: [msg("m1", "ann", "hi", 1)] },
			{ type: "updated", id: "m1", content: "hey", failed: false },
			{ type: "messages", messages: [msg("m1", "ann", "hey", 1)] },
		);
		expect(s.messages[0].edited).toBe(true);
	});

	it("closes the chat when we leave from elsewhere", () => {
		const s = apply(initialChatState(room), { type: "left", userId: "me" });
		expect(s.closed).toBeTruthy();
		const other = apply(initialChatState(room), {
			type: "left",
			userId: "ann",
		});
		expect(other.closed).toBeUndefined();
	});
});
