import { describe, expect, it } from "vitest";
import {
	base64UrlDecode,
	base64UrlEncode,
	parseDeviceCode,
	parseRoomKey,
	RoomKeyError,
	roomKeyFromHash,
	sealRoomKey,
} from "./room-key";

const roomKey = "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8"; // bytes 0..31
const devicePublicKey = "E75P6uryBMf9M1j8nAByGIHRdCeBKCJ-xnTzf3_pe20";

describe("base64url", () => {
	it("round-trips bytes that need - and _", () => {
		const bytes = new Uint8Array([0xfb, 0xff, 0xbf, 0x00, 0x01]);
		const encoded = base64UrlEncode(bytes);
		expect(encoded).not.toMatch(/[+/=]/);
		expect(Array.from(base64UrlDecode(encoded))).toEqual(Array.from(bytes));
	});
});

describe("parseRoomKey", () => {
	it("decodes a 32-byte key", () => {
		expect(Array.from(parseRoomKey(roomKey))).toEqual(
			Array.from({ length: 32 }, (_, i) => i),
		);
	});

	it.each(["", "short", "not base64!", `${roomKey}AA`])("rejects %j", (key) => {
		expect(() => parseRoomKey(key)).toThrow(RoomKeyError);
	});
});

describe("roomKeyFromHash", () => {
	it("reads the key fragment of an invite link", () => {
		expect(roomKeyFromHash(`#key=${roomKey}`)).toBe(roomKey);
		expect(roomKeyFromHash("")).toBe("");
		expect(roomKeyFromHash("#other=1")).toBe("");
	});
});

describe("parseDeviceCode", () => {
	it("splits the user id and public key", () => {
		const code = parseDeviceCode(` user-1.${devicePublicKey} `);
		expect(code.userId).toBe("user-1");
		expect(code.publicKey).toHaveLength(32);
	});

	it.each(["", "user-1", `.${devicePublicKey}`, "user-1.abc"])(
		"rejects %j",
		(code) => {
			expect(() => parseDeviceCode(code)).toThrow(RoomKeyError);
		},
	);
});

describe("sealRoomKey", () => {
	// Opening is tested on the Go side (sdk/go/roomkey_test.go), with a key
	// sealed by this function.
	it("produces a fresh, fixed-size sealed key that hides the room key", async () => {
		const key = parseRoomKey(roomKey);
		const pub = parseDeviceCode(`u.${devicePublicKey}`).publicKey;
		const a = await sealRoomKey(key, pub);
		const b = await sealRoomKey(key, pub);

		expect(base64UrlDecode(a)).toHaveLength(32 + 12 + 32 + 16);
		expect(a).not.toBe(b);
		expect(a).not.toContain(roomKey);
	});
});
