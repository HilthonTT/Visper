import nacl from "tweetnacl";
import { parseRoomKey, RoomKeyError } from "./room-key";

/**
 * Message and file encryption with the room key: NaCl secretbox
 * (XSalsa20-Poly1305), the same wire format as the Go SDK
 * (sdk/go/crypto.go, sdk/go/roomkey.go):
 *
 *   text: base64( nonce (24) || secretbox(plaintext) )
 *   file: "VISPER\0\x01" || nonce (24) || secretbox(content)
 */

export type RoomKey = Uint8Array<ArrayBuffer>;

const NONCE_SIZE = nacl.secretbox.nonceLength; // 24
const FILE_MAGIC = new Uint8Array([0x56, 0x49, 0x53, 0x50, 0x45, 0x52, 0, 1]); // "VISPER\0\x01"

/** Bytes encryption adds to a file. */
export const ENCRYPTED_FILE_OVERHEAD =
	FILE_MAGIC.length + NONCE_SIZE + nacl.secretbox.overheadLength;

/** Largest file the API stores, after encryption. */
export const MAX_UPLOAD_SIZE = 5 << 20;

export class DecryptionError extends Error {}

function toBase64(bytes: Uint8Array): string {
	let binary = "";
	for (const b of bytes) {
		binary += String.fromCharCode(b);
	}
	return btoa(binary);
}

function fromBase64(s: string): Uint8Array {
	try {
		return Uint8Array.from(atob(s), (c) => c.charCodeAt(0));
	} catch {
		throw new DecryptionError("invalid base64");
	}
}

/** Encrypts message text (or a reaction) with the room key. */
export function encryptText(plaintext: string, key: RoomKey): string {
	const nonce = nacl.randomBytes(NONCE_SIZE);
	const box = nacl.secretbox(new TextEncoder().encode(plaintext), nonce, key);
	const out = new Uint8Array(NONCE_SIZE + box.length);
	out.set(nonce);
	out.set(box, NONCE_SIZE);
	return toBase64(out);
}

/** Decrypts text encrypted by encryptText (or the Go SDK's Encrypt). */
export function decryptText(ciphertext: string, key: RoomKey): string {
	const data = fromBase64(ciphertext);
	if (data.length < NONCE_SIZE + nacl.secretbox.overheadLength) {
		throw new DecryptionError("ciphertext too short");
	}
	const opened = nacl.secretbox.open(
		data.subarray(NONCE_SIZE),
		data.subarray(0, NONCE_SIZE),
		key,
	);
	if (!opened) {
		throw new DecryptionError("decryption failed");
	}
	return new TextDecoder().decode(opened);
}

/**
 * Decrypts a field that may or may not be encrypted; failures read as a
 * placeholder rather than throwing, like the CLI.
 */
export function readField(
	value: string,
	encrypted: boolean,
	key: RoomKey,
): { text: string; failed: boolean } {
	if (!encrypted) {
		return { text: value, failed: false };
	}
	try {
		return { text: decryptText(value, key), failed: false };
	} catch {
		return { text: "[Decryption failed]", failed: true };
	}
}

/** Reports whether data starts like an encrypted file. */
export function isEncryptedFile(data: Uint8Array): boolean {
	return (
		data.length >= FILE_MAGIC.length &&
		FILE_MAGIC.every((b, i) => data[i] === b)
	);
}

/** Encrypts a file's content with the room key. */
export function encryptFile(plaintext: Uint8Array, key: RoomKey): Uint8Array {
	const nonce = nacl.randomBytes(NONCE_SIZE);
	const box = nacl.secretbox(plaintext, nonce, key);
	const out = new Uint8Array(FILE_MAGIC.length + NONCE_SIZE + box.length);
	out.set(FILE_MAGIC);
	out.set(nonce, FILE_MAGIC.length);
	out.set(box, FILE_MAGIC.length + NONCE_SIZE);
	return out;
}

/** Decrypts a file encrypted by encryptFile (or the Go SDK's EncryptFile). */
export function decryptFile(data: Uint8Array, key: RoomKey): Uint8Array {
	if (!isEncryptedFile(data) || data.length < ENCRYPTED_FILE_OVERHEAD) {
		throw new DecryptionError("not an encrypted file");
	}
	const start = FILE_MAGIC.length;
	const opened = nacl.secretbox.open(
		data.subarray(start + NONCE_SIZE),
		data.subarray(start, start + NONCE_SIZE),
		key,
	);
	if (!opened) {
		throw new DecryptionError("decryption failed");
	}
	return opened;
}

// ---------------------------------------------------------------------------
// Invite codes

export class InviteCodeError extends Error {}

/**
 * Splits an invite code ("<join code>.<base64url key>", see the Go SDK's
 * ParseInviteCode) into its join code and key.
 */
export function parseInviteCode(code: string): {
	joinCode: string;
	key: RoomKey;
} {
	const trimmed = code.trim();
	const at = trimmed.indexOf(".");
	const joinCode = (at < 0 ? trimmed : trimmed.slice(0, at)).toUpperCase();
	const keyPart = at < 0 ? "" : trimmed.slice(at + 1);
	if (!joinCode) {
		throw new InviteCodeError("Paste an invite code first.");
	}
	if (!keyPart) {
		throw new InviteCodeError(
			"That's only the join code. Ask for the full invite code (ctrl+p in their room): it carries the room's key.",
		);
	}
	try {
		return { joinCode, key: parseRoomKey(keyPart) };
	} catch (e) {
		if (e instanceof RoomKeyError) {
			throw new InviteCodeError(
				"That invite code is incomplete or mistyped. Copy it again.",
			);
		}
		throw e;
	}
}

/** Decodes a base64url room key (as found in an invite link's fragment). */
export function roomKeyFromUrl(key: string): RoomKey {
	return parseRoomKey(key);
}

// ---------------------------------------------------------------------------
// Image messages (mirrors clients/cli/internal/tui/chat_images.go)

const IMAGE_EXTENSIONS = [".jpg", ".jpeg", ".png", ".gif", ".webp", ".bmp"];
const ENCRYPTED_FILE_EXT = ".enc";

const isHttpUrl = (lower: string) =>
	lower.startsWith("http://") || lower.startsWith("https://");
const hasImageExtension = (lower: string) =>
	IMAGE_EXTENSIONS.some((ext) => lower.endsWith(ext));

/**
 * An encrypted image message: the stored file's URL (ending in ".enc") with
 * the image's file name as the fragment.
 */
export function isEncryptedImageUrl(content: string): boolean {
	const lower = content.toLowerCase();
	const hash = lower.indexOf("#");
	if (hash < 0) {
		return false;
	}
	const file = lower.slice(0, hash);
	return (
		isHttpUrl(file) &&
		file.endsWith(ENCRYPTED_FILE_EXT) &&
		hasImageExtension(lower.slice(hash + 1))
	);
}

/** A plain image link (e.g. pasted from the web). */
export function isPlainImageUrl(content: string): boolean {
	const file = content.toLowerCase().split("#")[0];
	return isHttpUrl(file) && hasImageExtension(file) && !/\s/.test(file);
}

/** The API's limit on a message's (encrypted) content, in characters. */
const MAX_CONTENT_LENGTH = 1000;

/**
 * Longest message plaintext, in UTF-8 bytes, whose encryptText output fits
 * the API's limit: base64(nonce || secretbox(plaintext)). 710 bytes.
 */
export const MAX_MESSAGE_BYTES =
	Math.floor(MAX_CONTENT_LENGTH / 4) * 3 -
	NONCE_SIZE -
	nacl.secretbox.overheadLength;

/** Whether text is short enough to be sent once encrypted. */
export function fitsMessage(text: string): boolean {
	return new TextEncoder().encode(text).length <= MAX_MESSAGE_BYTES;
}

/**
 * The message content for an encrypted image uploaded to fileUrl. A long file
 * name is shortened (keeping its extension) so the message still fits.
 */
export function encryptedImageContent(fileUrl: string, name: string): string {
	const budget = MAX_MESSAGE_BYTES - fileUrl.length - 1;
	let encoded = encodeURIComponent(name);
	if (encoded.length > budget) {
		const dot = name.lastIndexOf(".");
		const ext = dot > 0 ? name.slice(dot) : "";
		const base = [...(dot > 0 ? name.slice(0, dot) : name)];
		while (
			base.length > 0 &&
			encodeURIComponent(base.join("") + ext).length > budget
		) {
			base.pop();
		}
		encoded = encodeURIComponent((base.join("") || "image") + ext);
	}
	return `${fileUrl}#${encoded}`;
}

/** The file name in an encrypted image message's fragment. */
export function imageName(content: string): string {
	const hash = content.indexOf("#");
	const raw = hash < 0 ? content : content.slice(hash + 1);
	try {
		return decodeURIComponent(raw);
	} catch {
		return raw;
	}
}

/** The MIME type for an image file name. */
export function imageMimeType(name: string): string {
	const ext = name.toLowerCase().split(".").pop() ?? "";
	return (
		{
			jpg: "image/jpeg",
			jpeg: "image/jpeg",
			png: "image/png",
			gif: "image/gif",
			webp: "image/webp",
			bmp: "image/bmp",
		}[ext] ?? "application/octet-stream"
	);
}
