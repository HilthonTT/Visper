import { describe, expect, it } from "vitest";
import type { ApiFileInfo, ApiRoom } from "./chat-api";
import { type ChatMessage, initialChatState } from "./chat-state";
import { describeFiles, formatSize } from "./room-files";

const room: ApiRoom = {
	id: "r1",
	join_code: "ABC123",
	owner: { id: "ann", username: "Ann" },
	created_at: "2026-01-01T00:00:00Z",
	expires_at: "2026-01-02T00:00:00Z",
	members: [
		{ id: "ann", username: "Ann" },
		{ id: "bo", username: "Bo" },
	],
	current_user: { id: "me", username: "Me" },
	qr_code_url: "",
};

const file = (
	id: string,
	uploader: string,
	createdAt: string,
	username = "",
): ApiFileInfo => ({
	id,
	filename: "file.enc",
	size: 2048,
	url: `https://api.example/api/v1/d/r1/${id}.enc`,
	createdAt,
	uploader: { id: uploader, ...(username ? { username } : {}) },
});

const message = (id: string, content: string, failed = false): ChatMessage => ({
	id,
	userId: "bo",
	username: "Bo",
	content,
	failed,
	createdAt: 0,
	reactions: [],
});

describe("describeFiles", () => {
	const files = [
		file("f1", "bo", "2026-01-01T10:00:00Z"),
		file("f2", "me", "2026-01-01T11:00:00Z", "Me"),
		file("f3", "gone", "2026-01-01T09:00:00Z"),
	];

	it("names files from the messages that share them, newest first", () => {
		const state = {
			...initialChatState(room),
			messages: [
				message("m1", "https://api.example/api/v1/d/r1/f1.enc#cat%20pic.png"),
				// Undecryptable messages tell us nothing.
				message("m2", "https://api.example/api/v1/d/r1/f3.enc#x.png", true),
			],
		};
		const rows = describeFiles(files, state);
		expect(rows.map((r) => r.id)).toEqual(["f2", "f1", "f3"]);
		expect(rows[1]).toMatchObject({
			name: "cat pic.png",
			messageId: "m1",
			uploaderName: "Bo",
		});
		expect(rows[0]).toMatchObject({ name: "", uploaderName: "Me" });
		expect(rows[2]).toMatchObject({ name: "", uploaderName: "someone" });
	});

	it("lets members delete their own files and the owner delete any", () => {
		const mine = describeFiles(files, initialChatState(room));
		expect(mine.map((r) => r.canDelete)).toEqual([true, false, false]);

		const owner = describeFiles(
			files,
			initialChatState({ ...room, current_user: room.owner }),
		);
		expect(owner.every((r) => r.canDelete)).toBe(true);
	});
});

describe("formatSize", () => {
	it("picks a unit", () => {
		expect(formatSize(512)).toBe("512 B");
		expect(formatSize(1536)).toBe("1.5 KB");
		expect(formatSize(3.25 * 1024 * 1024)).toBe("3.3 MB");
	});
});
