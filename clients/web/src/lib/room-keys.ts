import nacl from "tweetnacl";
import {
	DecryptionError,
	decryptBytes,
	decryptFile,
	encryptBytes,
	encryptFile,
	imageMessageContent,
	newRoomKey,
	type RoomKey,
} from "./room-crypto";
import {
	base64UrlDecode,
	base64UrlEncode,
	openRoomKey,
	RoomKeyError,
	sealRoomKey,
} from "./room-key";

/*
 * Signed envelopes, key cards and rekeys: the same bytes as the Go SDK
 * (sdk/go/envelope.go, sdk/go/roomcrypto.go). Both test suites open
 * sdk/go/testdata/crypto-vectors.json, so keep them in step.
 *
 * Everything a member encrypts is a signed envelope, sealed with the room key:
 *
 *   secretbox( 0x02 || Ed25519 signature (64) || envelope JSON )
 *
 *   {"v":2,"k":"m"|"r"|"c","f":"<sender>","m":"<message ID>",
 *    "n":"<random nonce>","ts":<unix ms>,"t":"<text>","h":"<file hash>"}
 *
 * The room key keeps the server out; the signature, checked against the
 * sender's key card, keeps members from writing as each other. "n" and "ts"
 * catch replays, "h" binds an image message to its file.
 */

const ENVELOPE_VERSION = 2;
const SEALED_VERSION = 0x02;
const SIGNATURE_SIZE = nacl.sign.signatureLength; // 64
const NONCE_SIZE = nacl.secretbox.nonceLength; // 24
const CODE_SEPARATOR = ".";
const REKEY_CONTEXT = "visper/rekey/v1";

/** The API's limit on a message's (encrypted) content, in characters. */
export const MAX_CONTENT_LENGTH = 4000;

/** Largest signed envelope whose ciphertext fits MAX_CONTENT_LENGTH. */
export const MAX_SEALED_BYTES =
	Math.floor(MAX_CONTENT_LENGTH / 4) * 3 -
	NONCE_SIZE -
	nacl.secretbox.overheadLength -
	1 -
	SIGNATURE_SIZE;

/** How far an envelope's time may be from when the server received it. */
export const MAX_CLOCK_SKEW_MS = 15 * 60 * 1000;

/** Content that is not signed by its claimed sender or doesn't match it. */
export class UnverifiedError extends Error {}
/** No key card is known for the content's sender. */
export class UnknownSenderError extends UnverifiedError {}
/** Genuine content, replayed or rolled back. */
export class ReplayedError extends UnverifiedError {}
/** A downloaded file is not the one its message names. */
export class FileMismatchError extends Error {}
/** A rekey not signed by the room's owner. */
export class BadRekeyError extends Error {}

/** The placeholder shown for content that failed to decrypt or verify. */
export function failedText(e: unknown): string {
	if (e instanceof ReplayedError) {
		return "[Replayed message]";
	}
	return e instanceof UnverifiedError
		? "[Unverified message]"
		: "[Decryption failed]";
}

type Kind = "m" | "r" | "c";

type Envelope = {
	kind: Kind;
	from: string;
	msg: string;
	nonce: string;
	ts: number;
	text: string;
	fileHash: string;
};

/** The envelope JSON, fields in the Go SDK's order. */
function envelopeJson(e: Envelope): string {
	const out: Record<string, unknown> = { v: ENVELOPE_VERSION, k: e.kind };
	out.f = e.from;
	if (e.msg) {
		out.m = e.msg;
	}
	out.n = e.nonce;
	out.ts = e.ts;
	out.t = e.text;
	if (e.fileHash) {
		out.h = e.fileHash;
	}
	return JSON.stringify(out);
}

function newEnvelope(
	kind: Kind,
	from: string,
	msg: string,
	text: string,
	fileHash: string,
	now = Date.now(),
): Envelope {
	return {
		kind,
		from,
		msg,
		nonce: base64UrlEncode(nacl.randomBytes(16)),
		ts: now,
		text,
		fileHash,
	};
}

const utf8 = new TextEncoder();

function concat(...parts: Uint8Array[]): Uint8Array {
	const out = new Uint8Array(parts.reduce((n, p) => n + p.length, 0));
	let at = 0;
	for (const p of parts) {
		out.set(p, at);
		at += p.length;
	}
	return out;
}

/** Signs env with signSecretKey and encrypts it with key. */
function sealEnvelope(
	env: Envelope,
	signSecretKey: Uint8Array,
	key: RoomKey,
): string {
	const body = utf8.encode(envelopeJson(env));
	if (body.length > MAX_SEALED_BYTES) {
		throw new Error("That message is too long.");
	}
	const sig = nacl.sign.detached(body, signSecretKey);
	return encryptBytes(concat(Uint8Array.of(SEALED_VERSION), sig, body), key);
}

type Opened = { env: Envelope; sig: Uint8Array; body: Uint8Array };

/** Decrypts a signed envelope; its signature is still to be checked. */
function openEnvelope(ciphertext: string, key: RoomKey): Opened {
	const raw = decryptBytes(ciphertext, key);
	// Unsigned (version 1) envelopes are refused: accepting them would let
	// any member drop the signature and write as someone else.
	if (raw.length < 1 + SIGNATURE_SIZE || raw[0] !== SEALED_VERSION) {
		throw new UnverifiedError("not a signed envelope");
	}
	const sig = raw.slice(1, 1 + SIGNATURE_SIZE);
	const body = raw.slice(1 + SIGNATURE_SIZE);
	let e: Record<string, unknown>;
	try {
		const parsed: unknown = JSON.parse(new TextDecoder().decode(body));
		e = (typeof parsed === "object" && parsed !== null ? parsed : {}) as Record<
			string,
			unknown
		>;
	} catch {
		throw new UnverifiedError("not a signed envelope");
	}
	const str = (v: unknown) => (typeof v === "string" ? v : null);
	const kind = e.k;
	if (
		e.v !== ENVELOPE_VERSION ||
		(kind !== "m" && kind !== "r" && kind !== "c") ||
		!str(e.f) ||
		!str(e.n) ||
		typeof e.ts !== "number" ||
		str(e.t) === null ||
		(e.m !== undefined && str(e.m) === null) ||
		(e.h !== undefined && str(e.h) === null)
	) {
		throw new UnverifiedError("not a signed envelope");
	}
	return {
		env: {
			kind,
			from: e.f as string,
			msg: (e.m as string | undefined) ?? "",
			nonce: e.n as string,
			ts: e.ts,
			text: e.t as string,
			fileHash: (e.h as string | undefined) ?? "",
		},
		sig,
		body,
	};
}

/** Whether content written at `written` is plausibly what arrived at `at` (0: unknown). */
function near(written: number, at: number): boolean {
	return !at || Math.abs(written - at) <= MAX_CLOCK_SKEW_MS;
}

/** The base64url SHA-256 of an encrypted file, as image messages carry it. */
export async function fileHash(encrypted: Uint8Array): Promise<string> {
	const digest = await globalThis.crypto.subtle.digest(
		"SHA-256",
		new Uint8Array(encrypted),
	);
	return base64UrlEncode(new Uint8Array(digest));
}

// ---------------------------------------------------------------------------
// Identity

/**
 * This browser's keys for one session: a signing key (Ed25519) and a device
 * key (X25519) new room keys are sealed to. They live in memory only, like
 * the room key.
 */
export class Identity {
	private constructor(
		readonly userId: string,
		private readonly sign: nacl.SignKeyPair,
		private readonly device: nacl.BoxKeyPair,
	) {}

	static create(userId: string): Identity {
		return new Identity(userId, nacl.sign.keyPair(), nacl.box.keyPair());
	}

	/** An identity from fixed keys (for tests). */
	static fromSeeds(
		userId: string,
		signSeed: Uint8Array,
		devicePrivate: Uint8Array,
	): Identity {
		return new Identity(
			userId,
			nacl.sign.keyPair.fromSeed(signSeed),
			nacl.box.keyPair.fromSecretKey(devicePrivate),
		);
	}

	signingPublicKey(): string {
		return base64UrlEncode(this.sign.publicKey);
	}

	devicePublicKey(): string {
		return base64UrlEncode(this.device.publicKey);
	}

	/** @internal */
	signSecretKey(): Uint8Array {
		return this.sign.secretKey;
	}

	openRoomKey(sealed: string): Promise<RoomKey> {
		return openRoomKey(sealed, this.device.secretKey);
	}
}

// ---------------------------------------------------------------------------
// RoomCrypto

type MemberCard = { sign: Uint8Array; device: string };

export type Rekey = {
	epoch: number;
	keys: Record<string, string> | null;
	signature: string;
};

export type OpenedMessage = {
	text: string;
	/** Hash of the encrypted file an image message points to. */
	fileHash: string;
	edited: boolean;
};

export type ReadResult = { text: string; failed: boolean; fileHash: string };

function rekeyMessage(
	roomId: string,
	epoch: number,
	keys: Record<string, string>,
): string {
	let out = `${REKEY_CONTEXT}\n${roomId}\n${epoch}\n`;
	for (const id of Object.keys(keys).sort()) {
		out += `${id}=${keys[id]}\n`;
	}
	return out;
}

/**
 * One room's crypto state: our identity, the room's keys (the current one
 * and those it replaced, for older messages), the members' key cards and
 * what we have seen, to catch replays. It mirrors the Go SDK's RoomCrypto.
 *
 * A key card tells the room a member's signing and device keys: a signed
 * envelope of kind "c" whose text is "<signing key>.<device key>", sealed
 * with the room key (the server can't forge one) and stored by the API under
 * the uploader's ID (a member can't publish one for someone else).
 */
export class RoomCrypto {
	private keys: RoomKey[];
	private epochValue = 0;
	private readonly cards = new Map<string, MemberCard[]>();
	private readonly nonces = new Map<string, string>();
	private readonly versions = new Map<string, number>();
	private readonly departed = new Set<string>();

	/**
	 * `ownerId` is the room's owner as seen when entering the room, the only
	 * member whose rekeys are accepted.
	 */
	constructor(
		readonly me: Identity,
		readonly roomId: string,
		readonly ownerId: string,
		key: RoomKey,
	) {
		this.keys = [key];
	}

	/** The key new content is encrypted with. */
	currentKey(): RoomKey {
		return this.keys[this.keys.length - 1];
	}

	/** The rekey epoch of the current key, as far as we know. */
	epoch(): number {
		return this.epochValue;
	}

	isOwner(): boolean {
		return this.me.userId === this.ownerId;
	}

	private open(ciphertext: string): Opened {
		let first: unknown = new DecryptionError("decryption failed");
		for (const key of [...this.keys].reverse()) {
			try {
				return openEnvelope(ciphertext, key);
			} catch (e) {
				if (e instanceof UnverifiedError) {
					first = e;
				}
			}
		}
		throw first;
	}

	private verifySender(from: string, body: Uint8Array, sig: Uint8Array) {
		const cards = this.cards.get(from);
		if (!cards?.length) {
			throw new UnknownSenderError("no key card for the sender");
		}
		if (!cards.some((c) => nacl.sign.detached.verify(body, sig, c.sign))) {
			throw new UnverifiedError("bad signature");
		}
	}

	private openKind(ciphertext: string, kind: Kind, from: string): Envelope {
		const { env, sig, body } = this.open(ciphertext);
		if (env.kind !== kind || env.from !== from) {
			throw new UnverifiedError("sender or kind mismatch");
		}
		this.verifySender(from, body, sig);
		return env;
	}

	// Cards

	/** Our key card, sealed with the current key, to publish. */
	myCard(): string {
		const text =
			this.me.signingPublicKey() + CODE_SEPARATOR + this.me.devicePublicKey();
		return sealEnvelope(
			newEnvelope("c", this.me.userId, "", text, ""),
			this.me.signSecretKey(),
			this.currentKey(),
		);
	}

	/** Records a key card the API stores for userId; throws if it doesn't verify. */
	addCard(userId: string, sealed: string): void {
		const { env, sig, body } = this.open(sealed);
		if (env.kind !== "c" || env.from !== userId) {
			throw new UnverifiedError("card names someone else");
		}
		const at = env.text.indexOf(CODE_SEPARATOR);
		let sign: Uint8Array;
		let device: Uint8Array;
		try {
			sign = base64UrlDecode(env.text.slice(0, at));
			device = base64UrlDecode(env.text.slice(at + 1));
		} catch {
			throw new UnverifiedError("malformed card");
		}
		if (
			at < 0 ||
			sign.length !== nacl.sign.publicKeyLength ||
			device.length !== 32 ||
			!nacl.sign.detached.verify(body, sig, sign)
		) {
			throw new UnverifiedError("malformed card");
		}
		const deviceKey = env.text.slice(at + 1);
		const list = this.cards.get(userId) ?? [];
		const signKey = base64UrlEncode(sign);
		if (
			!list.some(
				(c) => c.device === deviceKey && base64UrlEncode(c.sign) === signKey,
			)
		) {
			list.push({ sign, device: deviceKey });
		}
		this.cards.set(userId, list);
	}

	/** Records every card that verifies; skips the rest. */
	addCards(cards: Record<string, string[]> | null | undefined): void {
		for (const [userId, list] of Object.entries(cards ?? {})) {
			for (const sealed of list ?? []) {
				try {
					this.addCard(userId, sealed);
				} catch {
					// Sealed with a key we never had, or forged.
				}
			}
		}
	}

	hasCard(userId: string): boolean {
		return (this.cards.get(userId)?.length ?? 0) > 0;
	}

	// Sealing

	sealMessage(text: string, fileHash = ""): string {
		return sealEnvelope(
			newEnvelope("m", this.me.userId, "", text, fileHash),
			this.me.signSecretKey(),
			this.currentKey(),
		);
	}

	sealEdit(messageId: string, text: string): string {
		return sealEnvelope(
			newEnvelope("m", this.me.userId, messageId, text, ""),
			this.me.signSecretKey(),
			this.currentKey(),
		);
	}

	sealReaction(messageId: string, emoji: string): string {
		return sealEnvelope(
			newEnvelope("r", this.me.userId, messageId, emoji, ""),
			this.me.signSecretKey(),
			this.currentKey(),
		);
	}

	/** The signed envelope's size for text, which must fit MAX_SEALED_BYTES. */
	sealedSize(messageId: string, text: string, fileHash = ""): number {
		return utf8.encode(
			envelopeJson(newEnvelope("m", this.me.userId, messageId, text, fileHash)),
		).length;
	}

	/** Whether text (as an edit of messageId, if given) fits once sealed. */
	fits(text: string, messageId = "", fileHash = ""): boolean {
		return this.sealedSize(messageId, text, fileHash) <= MAX_SEALED_BYTES;
	}

	/**
	 * The content of an encrypted image message for a file uploaded to
	 * fileUrl, its name shortened if needed so the message fits.
	 */
	imageContent(fileUrl: string, name: string, hash: string): string {
		const budget =
			MAX_SEALED_BYTES - this.sealedSize("", `${fileUrl}#`, hash) - 8;
		return imageMessageContent(fileUrl, name, budget);
	}

	// Opening

	/**
	 * Decrypts and verifies message messageId's content, shown as sent by
	 * `from`: a new message, or (in history) its latest edit. `created` and
	 * `updated` are the server's times (ms; 0 if unknown or never edited).
	 */
	openMessage(
		ciphertext: string,
		from: string,
		messageId: string,
		created: number,
		updated = 0,
	): OpenedMessage {
		const env = this.openKind(ciphertext, "m", from);
		if (env.msg === "") {
			// A new message needs a send time to check: with none, a server
			// could replay an old message as new just by leaving it out.
			if (!created || !near(env.ts, created)) {
				throw new ReplayedError("stale message");
			}
			this.claimNonce(env.nonce, messageId);
		} else if (env.msg === messageId) {
			if (!near(env.ts, updated || created)) {
				throw new ReplayedError("stale edit");
			}
		} else {
			throw new UnverifiedError("edit of another message");
		}
		this.advance(messageId, env.ts);
		return { text: env.text, fileHash: env.fileHash, edited: env.msg !== "" };
	}

	/**
	 * Decrypts and verifies an edit of messageId received at `at` (ms). The
	 * edit event doesn't name its author, so the sender is returned for the
	 * caller to compare with the message's author.
	 */
	openEdit(
		ciphertext: string,
		messageId: string,
		at: number,
	): OpenedMessage & { from: string } {
		const { env, sig, body } = this.open(ciphertext);
		if (env.kind !== "m" || env.msg !== messageId) {
			throw new UnverifiedError("not an edit of this message");
		}
		this.verifySender(env.from, body, sig);
		if (!near(env.ts, at)) {
			throw new ReplayedError("stale edit");
		}
		this.advance(messageId, env.ts);
		return {
			text: env.text,
			fileHash: env.fileHash,
			edited: true,
			from: env.from,
		};
	}

	openReaction(ciphertext: string, from: string, messageId: string): string {
		const env = this.openKind(ciphertext, "r", from);
		if (env.msg !== messageId) {
			throw new UnverifiedError("reaction to another message");
		}
		return env.text;
	}

	private claimNonce(nonce: string, messageId: string) {
		const seen = this.nonces.get(nonce);
		if (seen !== undefined && seen !== messageId) {
			throw new ReplayedError("message replayed under another ID");
		}
		this.nonces.set(nonce, messageId);
	}

	private advance(messageId: string, ts: number) {
		const shown = this.versions.get(messageId);
		if (shown !== undefined && ts < shown) {
			throw new ReplayedError("older than the version shown");
		}
		this.versions.set(messageId, ts);
	}

	/** openMessage that returns a placeholder instead of throwing. */
	readMessage(
		value: string,
		encrypted: boolean,
		from: string,
		messageId: string,
		created: number,
		updated = 0,
	): ReadResult {
		try {
			if (!encrypted) {
				// Every member encrypts: plaintext came from the server.
				throw new UnverifiedError("plaintext in an encrypted room");
			}
			const m = this.openMessage(value, from, messageId, created, updated);
			return { text: m.text, failed: false, fileHash: m.fileHash };
		} catch (e) {
			return { text: failedText(e), failed: true, fileHash: "" };
		}
	}

	/** openReaction that returns a placeholder instead of throwing. */
	readReaction(
		value: string,
		encrypted: boolean,
		from: string,
		messageId: string,
	): string {
		try {
			if (!encrypted) {
				throw new UnverifiedError("plaintext in an encrypted room");
			}
			return this.openReaction(value, from, messageId);
		} catch (e) {
			return failedText(e);
		}
	}

	// Files

	/** Encrypts a file with the current key; returns it with its hash. */
	async encryptFile(
		plaintext: Uint8Array,
	): Promise<{ data: Uint8Array<ArrayBuffer>; hash: string }> {
		const data = new Uint8Array(encryptFile(plaintext, this.currentKey()));
		return { data, hash: await fileHash(data) };
	}

	/** Checks a download against its message's hash and decrypts it. */
	async decryptFile(data: Uint8Array, hash: string): Promise<Uint8Array> {
		if (!hash || (await fileHash(data)) !== hash) {
			throw new FileMismatchError("the file does not match its message");
		}
		let last: unknown = new DecryptionError("decryption failed");
		for (const key of [...this.keys].reverse()) {
			try {
				return decryptFile(data, key);
			} catch (e) {
				last = e;
			}
		}
		throw last;
	}

	// Rekeying (see the Go SDK's RoomCrypto.NewRekey)

	/** Leaves userId out of our next rekey, whatever the server lists. */
	markDeparted(userId: string): void {
		this.departed.add(userId);
	}

	/**
	 * Forgets that userId departed, for when they rejoin; without it a member
	 * who left and came back was silently left out of every later rekey.
	 */
	clearDeparted(userId: string): void {
		this.departed.delete(userId);
	}

	/**
	 * Makes a new room key for `members` (owner only): the signed rekey to
	 * post, the new key, and the members without a card (who can't get it).
	 * Call commitKey once the API accepted it.
	 */
	async newRekey(
		members: string[],
		currentEpoch: number,
	): Promise<{ rekey: Rekey; newKey: RoomKey; missing: string[] }> {
		if (!this.isOwner()) {
			throw new BadRekeyError("only the room owner can change the room key");
		}
		const newKey = newRoomKey();
		const keys: Record<string, string> = {};
		const missing: string[] = [];
		for (const id of members) {
			if (id === this.me.userId || this.departed.has(id)) {
				continue;
			}
			const cards = this.cards.get(id);
			if (!cards?.length) {
				missing.push(id);
				continue;
			}
			// The latest card is the member's current device.
			keys[id] = await sealRoomKey(
				newKey,
				new Uint8Array(base64UrlDecode(cards[cards.length - 1].device)),
			);
		}
		const epoch = currentEpoch + 1;
		const sig = nacl.sign.detached(
			utf8.encode(rekeyMessage(this.roomId, epoch, keys)),
			this.me.signSecretKey(),
		);
		return {
			rekey: { epoch, keys, signature: base64UrlEncode(sig) },
			newKey,
			missing,
		};
	}

	/** Makes newKey (from newRekey) the current key at epoch. */
	commitKey(newKey: RoomKey, epoch: number): void {
		this.addKey(newKey);
		this.epochValue = Math.max(this.epochValue, epoch);
	}

	private addKey(key: RoomKey) {
		const id = base64UrlEncode(key);
		this.keys = this.keys.filter((k) => base64UrlEncode(k) !== id);
		this.keys.push(key);
	}

	/**
	 * Checks a rekey and, if it is newer and includes us, switches to its
	 * key. Returns whether the key changed (then publish a new card).
	 */
	async applyRekey(r: Rekey | null | undefined): Promise<boolean> {
		if (!r || r.epoch <= this.epochValue || this.isOwner()) {
			return false;
		}
		let sig: Uint8Array;
		try {
			sig = base64UrlDecode(r.signature);
			this.verifySender(
				this.ownerId,
				utf8.encode(rekeyMessage(this.roomId, r.epoch, r.keys ?? {})),
				sig,
			);
		} catch {
			throw new BadRekeyError(
				"the new room key was not signed by the room owner",
			);
		}
		const sealed = r.keys?.[this.me.userId];
		if (!sealed) {
			// From before we joined, or one that left us out.
			this.epochValue = Math.max(this.epochValue, r.epoch);
			return false;
		}
		let newKey: RoomKey;
		try {
			newKey = await this.me.openRoomKey(sealed);
		} catch (e) {
			throw e instanceof RoomKeyError ? new BadRekeyError(e.message) : e;
		}
		if (r.epoch <= this.epochValue) {
			return false; // applied meanwhile
		}
		this.addKey(newKey);
		this.epochValue = r.epoch;
		return true;
	}
}

/** @internal For the cross-implementation tests. */
export const __test = { rekeyMessage };
