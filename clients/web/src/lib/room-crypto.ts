import nacl from "tweetnacl";
import { base64UrlEncode, parseRoomKey, RoomKeyError } from "./room-key";

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

/*
 * Message text and reactions are sealed in an envelope naming their kind and
 * sender (and, for reactions and edits, the message), the same JSON as the
 * Go SDK (sdk/go/envelope.go):
 *
 *   {"v":1,"k":"m"|"r","f":"<sender user ID>","m":"<message ID>","t":"<text>"}
 *
 * The server relays the sender's ID in plaintext; checking it against the
 * sealed one stops it passing one member's words off as another's.
 */

export class UnverifiedError extends Error {}

type Kind = "m" | "r";

export type Envelope = { kind: Kind; from: string; msg: string; text: string };

/** The envelope JSON for text (the Go SDK omits an empty "m" too). */
function envelopeJson(kind: Kind, from: string, msg: string, text: string) {
	return JSON.stringify(
		msg
			? { v: 1, k: kind, f: from, m: msg, t: text }
			: { v: 1, k: kind, f: from, t: text },
	);
}

/** Encrypts message text from `from`; pass the message ID when editing it. */
export function sealMessage(
	text: string,
	from: string,
	messageId: string,
	key: RoomKey,
): string {
	return encryptText(envelopeJson("m", from, messageId, text), key);
}

/** Encrypts `from`'s reaction to messageId. */
export function sealReaction(
	emoji: string,
	from: string,
	messageId: string,
	key: RoomKey,
): string {
	return encryptText(envelopeJson("r", from, messageId, emoji), key);
}

/** Decrypts an envelope without checking whom it claims to be from. */
export function openEnvelope(ciphertext: string, key: RoomKey): Envelope {
	const plain = decryptText(ciphertext, key);
	let raw: unknown;
	try {
		raw = JSON.parse(plain);
	} catch {
		throw new UnverifiedError("not a sealed envelope");
	}
	const e = (typeof raw === "object" && raw !== null ? raw : {}) as Record<
		string,
		unknown
	>;
	if (
		e.v !== 1 ||
		(e.k !== "m" && e.k !== "r") ||
		typeof e.f !== "string" ||
		!e.f ||
		typeof e.t !== "string" ||
		(e.m !== undefined && typeof e.m !== "string")
	) {
		throw new UnverifiedError("not a sealed envelope");
	}
	return { kind: e.k, from: e.f, msg: (e.m as string) ?? "", text: e.t };
}

export type ReadResult = { text: string; failed: boolean };

/** The placeholder shown for content that failed to decrypt or verify. */
export function failedText(e: unknown): string {
	return e instanceof UnverifiedError
		? "[Unverified message]"
		: "[Decryption failed]";
}

function readSealed(
	value: string,
	encrypted: boolean,
	key: RoomKey,
	check: (e: Envelope) => boolean,
): ReadResult {
	try {
		if (!encrypted) {
			// Every member of a room encrypts; plaintext came from the server.
			throw new UnverifiedError("plaintext in an encrypted room");
		}
		const env = openEnvelope(value, key);
		if (!check(env)) {
			throw new UnverifiedError("sender or message mismatch");
		}
		return { text: env.text, failed: false };
	} catch (e) {
		return { text: failedText(e), failed: true };
	}
}

/**
 * Decrypts message messageId's content, shown as sent by `from`, checking
 * that `from` sealed it for that message (or as a new message). Failures read
 * as a placeholder rather than throwing, like the CLI.
 */
export function readMessage(
	value: string,
	encrypted: boolean,
	key: RoomKey,
	from: string,
	messageId: string,
): ReadResult {
	return readSealed(
		value,
		encrypted,
		key,
		(e) => e.kind === "m" && e.from === from && (!e.msg || e.msg === messageId),
	);
}

/** Decrypts `from`'s reaction to messageId, checking that it is one. */
export function readReaction(
	value: string,
	encrypted: boolean,
	key: RoomKey,
	from: string,
	messageId: string,
): ReadResult {
	return readSealed(
		value,
		encrypted,
		key,
		(e) => e.kind === "r" && e.from === from && e.msg === messageId,
	);
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

/**
 * A new random room key. Made by the client that creates the room and never
 * sent to the API (like the Go SDK's NewRoomKey).
 */
export function newRoomKey(): RoomKey {
	return new Uint8Array(nacl.randomBytes(nacl.secretbox.keyLength));
}

/** The invite code to share: "<join code>.<base64url key>". */
export function formatInviteCode(joinCode: string, key: RoomKey): string {
	return `${joinCode}.${base64UrlEncode(key)}`;
}

/**
 * The invite link to share: the room's join URL from the API with the key
 * in its fragment (`#key=…`), which browsers never send to a server. The
 * join code is replaced with `joinCode`, which may have been regenerated
 * since the URL was issued. Returns "" without a join URL.
 */
export function formatInviteLink(
	joinUrl: string,
	joinCode: string,
	key: RoomKey,
): string {
	if (!joinUrl) {
		return "";
	}
	let url: URL;
	try {
		url = new URL(joinUrl);
	} catch {
		return "";
	}
	url.searchParams.set("joinCode", joinCode);
	url.hash = new URLSearchParams({ key: base64UrlEncode(key) }).toString();
	return url.toString();
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
 * Largest sealed envelope, in UTF-8 bytes, whose encryptText output fits the
 * API's limit: base64(nonce || secretbox(envelope)). 710 bytes.
 */
export const MAX_MESSAGE_BYTES =
	Math.floor(MAX_CONTENT_LENGTH / 4) * 3 -
	NONCE_SIZE -
	nacl.secretbox.overheadLength;

function sealedSize(from: string, messageId: string, text: string): number {
	return new TextEncoder().encode(envelopeJson("m", from, messageId, text))
		.length;
}

/**
 * Whether text, sent by `from` (as an edit of messageId, if given), is short
 * enough once sealed and encrypted.
 */
export function fitsMessage(
	text: string,
	from: string,
	messageId = "",
): boolean {
	return sealedSize(from, messageId, text) <= MAX_MESSAGE_BYTES;
}

/**
 * The message content for an encrypted image uploaded to fileUrl, sent by
 * `from`. A long file name is shortened (keeping its extension) so the
 * message still fits; the encoded name is plain ASCII, so it costs its length.
 */
export function encryptedImageContent(
	fileUrl: string,
	name: string,
	from: string,
): string {
	const budget = MAX_MESSAGE_BYTES - sealedSize(from, "", `${fileUrl}#`);
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
