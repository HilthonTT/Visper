import { describe, expect, it } from "vitest";
import {
	DecryptionError,
	decryptFile,
	decryptText,
	ENCRYPTED_FILE_OVERHEAD,
	encryptFile,
	encryptText,
	formatInviteCode,
	formatInviteLink,
	InviteCodeError,
	imageMessageContent,
	imageMimeType,
	imageName,
	isAutoLoadImageUrl,
	isEncryptedFile,
	isEncryptedImageUrl,
	isPlainImageUrl,
	newRoomKey,
	parseInviteCode,
	type RoomKey,
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

	it.each([
		["https://api.visper.test/api/v1/d/r/f.enc#cat.png", true],
		["https://API.visper.test:8443/p/a.png", true],
		["https://tracker.example/p.png", false],
		["https://tracker.example/d/r/f.enc#cat.png", false],
		["https://api.visper.test.evil.example/p.png", false],
		["https://api.visper.test/notes.txt", false],
	])("auto-loads %s only from the API host: %s", (content, auto) => {
		expect(isAutoLoadImageUrl(content, "https://api.visper.test:5004")).toBe(
			auto,
		);
	});

	it("auto-loads nothing when the API address is unusable", () => {
		expect(isAutoLoadImageUrl("https://x/p/a.png", "")).toBe(false);
	});

	it("names the file in the fragment", () => {
		const content = imageMessageContent(
			"https://x/d/r/f.enc",
			"my cat.png",
			500,
		);
		expect(content).toBe("https://x/d/r/f.enc#my%20cat.png");
		expect(isEncryptedImageUrl(content)).toBe(true);
		expect(imageName(content)).toBe("my cat.png");
		expect(imageMimeType("my cat.PNG")).toBe("image/png");
	});

	it("shortens long names, keeping the extension", () => {
		const url = "https://example.com/files/abc.enc";
		const content = imageMessageContent(url, `${"長".repeat(400)}.png`, 100);
		expect(content.length - url.length - 1).toBeLessThanOrEqual(100);
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
