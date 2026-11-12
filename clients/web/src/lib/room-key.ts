/**
 * Room keys stay on the clients: the server only ever stores ciphertext.
 *
 * An invite link carries its room's key in the URL fragment (`#key=…`), which
 * browsers never send to a server. To hand that key to a CLI, the web client
 * seals it to the CLI's device key (from the device code on the CLI's
 * settings page) and the API relays the sealed key without being able to
 * read it.
 *
 * Sealed key = base64url(ephemeral X25519 public key (32) || nonce (12) ||
 * AES-256-GCM ciphertext), the AES key being HKDF-SHA256 of the X25519 shared
 * secret salted with both public keys. It mirrors `SealRoomKey` in the Go SDK
 * (sdk/go/roomkey.go); keep the two in step.
 */

const KEY_SIZE = 32;
const SEAL_INFO = "visper/sealed-room-key/v1";
const CODE_SEPARATOR = ".";

export class RoomKeyError extends Error {}

export function base64UrlEncode(bytes: Uint8Array): string {
	let binary = "";
	for (const b of bytes) {
		binary += String.fromCharCode(b);
	}
	return btoa(binary)
		.replace(/\+/g, "-")
		.replace(/\//g, "_")
		.replace(/=+$/, "");
}

export function base64UrlDecode(s: string): Uint8Array {
	if (!/^[A-Za-z0-9_-]*={0,2}$/.test(s)) {
		throw new RoomKeyError("not base64url");
	}
	const b64 = s.replace(/=+$/, "").replace(/-/g, "+").replace(/_/g, "/");
	const binary = atob(b64 + "=".repeat((4 - (b64.length % 4)) % 4));
	return Uint8Array.from(binary, (c) => c.charCodeAt(0));
}

/** Decodes a room key as found in an invite; throws a RoomKeyError if bad. */
export function parseRoomKey(key: string): Uint8Array<ArrayBuffer> {
	let bytes: Uint8Array;
	try {
		bytes = base64UrlDecode(key.trim());
	} catch {
		throw new RoomKeyError("The room key is not valid. Copy it again.");
	}
	if (bytes.length !== KEY_SIZE) {
		throw new RoomKeyError("The room key is incomplete. Copy it again.");
	}
	return new Uint8Array(bytes);
}

/** The room key in an invite link's fragment (`#key=…`), or "". */
export function roomKeyFromHash(hash: string): string {
	const params = new URLSearchParams(hash.replace(/^#/, ""));
	return params.get("key") ?? "";
}

export type DeviceCode = { userId: string; publicKey: Uint8Array<ArrayBuffer> };

/** Splits a CLI device code (`<user id>.<public key>`); throws if invalid. */
export function parseDeviceCode(code: string): DeviceCode {
	const trimmed = code.trim();
	const at = trimmed.indexOf(CODE_SEPARATOR);
	const userId = at > 0 ? trimmed.slice(0, at) : "";
	let publicKey: Uint8Array | null = null;
	try {
		publicKey = at > 0 ? base64UrlDecode(trimmed.slice(at + 1)) : null;
	} catch {
		publicKey = null;
	}
	if (!userId || !publicKey || publicKey.length !== 32) {
		throw new RoomKeyError(
			"That device code is incomplete. Copy it again from your CLI's settings page.",
		);
	}
	return { userId, publicKey: new Uint8Array(publicKey) };
}

/** Encrypts a room key to a device's X25519 public key. */
export async function sealRoomKey(
	roomKey: Uint8Array<ArrayBuffer>,
	recipientPublicKey: Uint8Array<ArrayBuffer>,
): Promise<string> {
	if (roomKey.length !== KEY_SIZE) {
		throw new RoomKeyError("invalid room key");
	}
	const { subtle } = globalThis.crypto;

	const recipient = await subtle.importKey(
		"raw",
		recipientPublicKey,
		{ name: "X25519" },
		false,
		[],
	);
	const ephemeral = (await subtle.generateKey({ name: "X25519" }, true, [
		"deriveBits",
	])) as CryptoKeyPair;
	const ephemeralPublic = new Uint8Array(
		await subtle.exportKey("raw", ephemeral.publicKey),
	);
	const shared = await subtle.deriveBits(
		{ name: "X25519", public: recipient },
		ephemeral.privateKey,
		256,
	);

	const salt = new Uint8Array(64);
	salt.set(ephemeralPublic, 0);
	salt.set(recipientPublicKey, 32);
	const hkdfKey = await subtle.importKey("raw", shared, "HKDF", false, [
		"deriveKey",
	]);
	const aesKey = await subtle.deriveKey(
		{
			name: "HKDF",
			hash: "SHA-256",
			salt,
			info: new TextEncoder().encode(SEAL_INFO),
		},
		hkdfKey,
		{ name: "AES-GCM", length: 256 },
		false,
		["encrypt"],
	);

	const nonce = globalThis.crypto.getRandomValues(new Uint8Array(12));
	const ciphertext = new Uint8Array(
		await subtle.encrypt({ name: "AES-GCM", iv: nonce }, aesKey, roomKey),
	);

	const out = new Uint8Array(32 + 12 + ciphertext.length);
	out.set(ephemeralPublic, 0);
	out.set(nonce, 32);
	out.set(ciphertext, 44);
	return base64UrlEncode(out);
}
