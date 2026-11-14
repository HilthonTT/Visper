import { describe, expect, it } from "vitest";
import {
	DecryptionError,
	decryptFile,
	decryptText,
	ENCRYPTED_FILE_OVERHEAD,
	encryptedImageContent,
	encryptFile,
	encryptText,
	fitsMessage,
	formatInviteCode,
	formatInviteLink,
	InviteCodeError,
	imageMimeType,
	imageName,
	isEncryptedFile,
	isEncryptedImageUrl,
	isPlainImageUrl,
	MAX_MESSAGE_BYTES,
	newRoomKey,
	parseInviteCode,
	type RoomKey,
	readMessage,
	readReaction,
	sealMessage,
	sealReaction,
} from "./room-crypto";

// Bytes 0..31, base64url; the vectors below were made with the Go SDK
// (EncryptWithKeyB64 and EncryptFile) with this key.
const KEY_URL = "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8";
const key = new Uint8Array(32).map((_, i) => i) as RoomKey;
const otherKey = new Uint8Array(32).fill(9) as RoomKey;
const GO_TEXT =
	"iYXFLVqyqYb66mcqfKYVZj/r4s+uU1mzSLAYXZ//Xa+M1ZsMG0/6k8+EQQdj6alFiYrMRXgVPtb40DM=";
const GO_FILE =
	"VklTUEVSAAFONcjDyuCfiX5FiAZsah3jLlbjnft/h/hbodI4Ke6prHax+iw0OUh7cBUom86bTyQ5UlWgeiUU";

const ME = "629bfa91-c2d7-8ec5-b29e-d701d23d339d";

const fromB64 = (s: string) => Uint8Array.from(atob(s), (c) => c.charCodeAt(0));

describe("text encryption", () => {
	it("decrypts text encrypted by the Go SDK", () => {
		expect(decryptText(GO_TEXT, key)).toBe("héllo from Go 👋");
	});

	it("round-trips with a fresh nonce each time", () => {
		const a = encryptText("hi there", key);
		const b = encryptText("hi there", key);
		expect(a).not.toBe(b);
		expect(decryptText(a, key)).toBe("hi there");
	});

	it("rejects the wrong key and garbage", () => {
		expect(() => decryptText(GO_TEXT, otherKey)).toThrow(DecryptionError);
		expect(() => decryptText("not base64!", key)).toThrow(DecryptionError);
		expect(() => decryptText("AAAA", key)).toThrow(DecryptionError);
	});

	it("reads failed fields as placeholders without throwing", () => {
		expect(readMessage(GO_TEXT, true, otherKey, "alice", "m1")).toEqual({
			text: "[Decryption failed]",
			failed: true,
		});
		// Plaintext in an encrypted room can only have come from the server.
		expect(readMessage("plain", false, key, "alice", "m1")).toEqual({
			text: "[Unverified message]",
			failed: true,
		});
	});
});

describe("file encryption", () => {
	it("decrypts a file encrypted by the Go SDK", () => {
		const plain = decryptFile(fromB64(GO_FILE), key);
		expect(plain[0]).toBe(0x89);
		expect(new TextDecoder().decode(plain.subarray(1))).toBe("PNG fake image");
	});

	it("round-trips with the Visper header", () => {
		const data = new TextEncoder().encode("image bytes");
		const enc = encryptFile(data, key);
		expect(isEncryptedFile(enc)).toBe(true);
		expect(enc.length).toBe(data.length + ENCRYPTED_FILE_OVERHEAD);
		expect(Array.from(decryptFile(enc, key))).toEqual(Array.from(data));
		expect(() => decryptFile(enc, otherKey)).toThrow(DecryptionError);
		expect(() => decryptFile(data, key)).toThrow(DecryptionError);
	});
});

describe("parseInviteCode", () => {
	it("splits the join code and key", () => {
		const { joinCode, key: k } = parseInviteCode(` abc123.${KEY_URL}\n`);
		expect(joinCode).toBe("ABC123");
		expect(Array.from(k)).toEqual(Array.from(key));
	});

	it.each([
		["", "Paste an invite code"],
		["ABC123", "only the join code"],
		["ABC123.", "only the join code"],
		["ABC123.short", "incomplete"],
	])("rejects %j", (code, message) => {
		expect(() => parseInviteCode(code)).toThrow(InviteCodeError);
		expect(() => parseInviteCode(code)).toThrow(message);
	});
});

// Sealed by the Go SDK (SealMessage / SealReaction) with key 0..31.
const GO_SEALED_MESSAGE =
	"KZWsujdAPWCwXC52EPrqpF8JRYOppUvBxUC+HYAMI9gR/KKrqlIu6Qv+AcN+BS6WxYgzetJNnYOtkLJGbbgp7YsL+VMya75VovRuHOhvuR7I32sdvgXp";
const GO_SEALED_REACTION =
	"WIgYJ3Hhf/K5KuqTI4hPRhhp6WIOW8Sgyl0wXVJJHovmGdgvk17Ok1tFs7D2KteLmuYoKpP2i7xyKF5DHPPGXhOlLLLqhX7AzNQTa41C8lH4RyMoI5vA";

describe("sealed envelopes", () => {
	it("opens what the Go SDK sealed", () => {
		expect(readMessage(GO_SEALED_MESSAGE, true, key, "alice", "m9")).toEqual({
			text: "hello from go",
			failed: false,
		});
		expect(readReaction(GO_SEALED_REACTION, true, key, "alice", "m1")).toEqual({
			text: "👍",
			failed: false,
		});
	});

	it("rejects content relabelled by the server", () => {
		const unverified = { text: "[Unverified message]", failed: true };
		const msg = sealMessage("hi", "alice", "", key);
		expect(readMessage(msg, true, key, "alice", "m1").text).toBe("hi");
		expect(readMessage(msg, true, key, "bob", "m1")).toEqual(unverified);

		const edit = sealMessage("edited", "alice", "m1", key);
		expect(readMessage(edit, true, key, "alice", "m2")).toEqual(unverified);

		const reaction = sealReaction("👍", "alice", "m1", key);
		expect(readMessage(reaction, true, key, "alice", "m1")).toEqual(unverified);
		expect(readReaction(reaction, true, key, "alice", "m2")).toEqual(
			unverified,
		);
		expect(readReaction(reaction, true, key, "bob", "m1")).toEqual(unverified);

		// Bare ciphertext without an envelope is not trusted either.
		expect(
			readMessage(encryptText("hi", key), true, key, "alice", "m1"),
		).toEqual(unverified);
	});
});

describe("image messages", () => {
	it.each([
		["https://x/api/v1/d/r/f.enc#cat.png", true, false],
		["https://x/api/v1/d/r/f.enc#My%20Cat.JPG", true, false],
		["https://x/api/v1/d/r/f.enc#notes.txt", false, false],
		["https://x/api/v1/d/r/f.enc", false, false],
		["https://x/a.txt#cat.png", false, false],
		["https://x/p/a.png", false, true],
		["ftp://x/a.png", false, false],
		["look at https://x/a.png", false, false],
	])("%s", (content, encrypted, plain) => {
		expect(isEncryptedImageUrl(content)).toBe(encrypted);
		expect(isPlainImageUrl(content)).toBe(plain);
	});

	it("names the file in the fragment", () => {
		const content = encryptedImageContent(
			"https://x/d/r/f.enc",
			"my cat.png",
			ME,
		);
		expect(content).toBe("https://x/d/r/f.enc#my%20cat.png");
		expect(isEncryptedImageUrl(content)).toBe(true);
		expect(imageName(content)).toBe("my cat.png");
		expect(imageMimeType("my cat.PNG")).toBe("image/png");
	});
});

describe("message size limit", () => {
	it("keeps the largest allowed message under the API's 1000 characters", () => {
		expect(
			encryptText("a".repeat(MAX_MESSAGE_BYTES), key).length,
		).toBeLessThanOrEqual(1000);
		expect(
			encryptText("a".repeat(MAX_MESSAGE_BYTES + 1), key).length,
		).toBeGreaterThan(1000);
	});

	it("counts the sealed envelope in UTF-8 bytes, not characters", () => {
		expect(fitsMessage("a".repeat(600), ME)).toBe(true);
		expect(fitsMessage("漢".repeat(300), ME)).toBe(false);
		// The largest text that fits still encrypts under the API's limit.
		let n = 0;
		while (fitsMessage("a".repeat(n + 1), ME)) {
			n++;
		}
		expect(sealMessage("a".repeat(n), ME, "", key).length).toBeLessThanOrEqual(
			1000,
		);
		expect(sealMessage("a".repeat(n + 1), ME, "", key).length).toBeGreaterThan(
			1000,
		);
	});

	it("shortens long image names so the message fits", () => {
		const url = "https://example.com/files/abc.enc";
		const content = encryptedImageContent(url, `${"長".repeat(400)}.png`, ME);
		expect(fitsMessage(content, ME)).toBe(true);
		expect(isEncryptedImageUrl(content)).toBe(true);
		expect(imageName(content).endsWith(".png")).toBe(true);
	});
});

describe("new rooms and invites", () => {
	it("makes distinct 32-byte room keys", () => {
		const a = newRoomKey();
		const b = newRoomKey();
		expect(a).toHaveLength(32);
		expect(a).not.toEqual(b);
	});

	it("formats an invite code that parseInviteCode reads back", () => {
		const key = newRoomKey();
		const code = formatInviteCode("ABC123", key);
		expect(code).toMatch(/^ABC123\.[A-Za-z0-9_-]{43}$/);
		expect(parseInviteCode(code)).toEqual({ joinCode: "ABC123", key });
	});

	it("puts the key in the invite link's fragment", () => {
		const key = newRoomKey();
		const link = formatInviteLink(
			"https://visper.example/?joinCode=OLD111&secureCode=s3cret",
			"NEW222",
			key,
		);
		const url = new URL(link);
		expect(url.searchParams.get("joinCode")).toBe("NEW222");
		expect(url.searchParams.get("secureCode")).toBe("s3cret");
		const fromHash = new URLSearchParams(url.hash.slice(1)).get("key") ?? "";
		expect(parseInviteCode(`NEW222.${fromHash}`).key).toEqual(key);
	});

	it("gives no link without a join URL", () => {
		expect(formatInviteLink("", "ABC123", newRoomKey())).toBe("");
		expect(formatInviteLink("not a url", "ABC123", newRoomKey())).toBe("");
	});
});
