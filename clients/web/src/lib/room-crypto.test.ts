import { describe, expect, it } from "vitest";
import {
	DecryptionError,
	decryptFile,
	decryptText,
	ENCRYPTED_FILE_OVERHEAD,
	encryptedImageContent,
	encryptFile,
	encryptText,
	InviteCodeError,
	imageMimeType,
	imageName,
	isEncryptedFile,
	isEncryptedImageUrl,
	isPlainImageUrl,
	parseInviteCode,
	type RoomKey,
	readField,
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

	it("reads plain and failed fields without throwing", () => {
		expect(readField("plain", false, key)).toEqual({
			text: "plain",
			failed: false,
		});
		expect(readField(GO_TEXT, true, otherKey).failed).toBe(true);
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

	it("names the file in the fragment", () => {
		const content = encryptedImageContent("https://x/d/r/f.enc", "my cat.png");
		expect(content).toBe("https://x/d/r/f.enc#my%20cat.png");
		expect(isEncryptedImageUrl(content)).toBe(true);
		expect(imageName(content)).toBe("my cat.png");
		expect(imageMimeType("my cat.PNG")).toBe("image/png");
	});
});
