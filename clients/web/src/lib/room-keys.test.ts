import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import { encryptText, newRoomKey, type RoomKey } from "./room-crypto";
import { base64UrlDecode } from "./room-key";
import {
	__test,
	BadRekeyError,
	FileMismatchError,
	failedText,
	Identity,
	MAX_CLOCK_SKEW_MS,
	MAX_CONTENT_LENGTH,
	MAX_SEALED_BYTES,
	ReplayedError,
	RoomCrypto,
	UnknownSenderError,
	UnverifiedError,
} from "./room-keys";

// Content sealed by the Go SDK with fixed keys (sdk/go/vectors_test.go).
type VectorIdentity = {
	user_id: string;
	sign_seed: string;
	device_private: string;
	sign_public: string;
	device_public: string;
};
type VectorMessage = {
	ciphertext: string;
	from: string;
	message_id: string;
	at: number;
	text: string;
	file_hash?: string;
};
type Vectors = {
	room_id: string;
	room_key: string;
	alice: VectorIdentity;
	bob: VectorIdentity;
	alice_card: string;
	bob_card: string;
	message: VectorMessage;
	edit: VectorMessage;
	reaction: VectorMessage;
	image: VectorMessage;
	file: { plaintext: string; encrypted: string; hash: string };
	rekey: { epoch: number; keys: Record<string, string>; signature: string };
	rekey_message: string;
	new_key: string;
};

const vectors: Vectors = JSON.parse(
	readFileSync(
		fileURLToPath(
			new URL(
				"../../../../sdk/go/testdata/crypto-vectors.json",
				import.meta.url,
			),
		),
		"utf8",
	),
);

const fromB64 = (s: string) =>
	Uint8Array.from(atob(s), (c) => c.charCodeAt(0)) as RoomKey;
const toB64 = (b: Uint8Array) => btoa(String.fromCharCode(...b));

function vectorIdentity(v: VectorIdentity): Identity {
	return Identity.fromSeeds(
		v.user_id,
		base64UrlDecode(v.sign_seed),
		base64UrlDecode(v.device_private),
	);
}

describe("compatibility with the Go SDK", () => {
	const bob = vectorIdentity(vectors.bob);
	const rc = new RoomCrypto(
		bob,
		vectors.room_id,
		vectors.alice.user_id,
		fromB64(vectors.room_key),
	);

	it("derives the same keys from the same seeds", () => {
		expect(bob.signingPublicKey()).toBe(vectors.bob.sign_public);
		expect(bob.devicePublicKey()).toBe(vectors.bob.device_public);
		const alice = vectorIdentity(vectors.alice);
		expect(alice.signingPublicKey()).toBe(vectors.alice.sign_public);
	});

	it("opens Go's cards, messages, edits, reactions, images and files", async () => {
		rc.addCard(vectors.alice.user_id, vectors.alice_card);
		rc.addCard(vectors.bob.user_id, vectors.bob_card);

		const { message, edit, reaction, image } = vectors;
		expect(
			rc.openMessage(
				message.ciphertext,
				message.from,
				message.message_id,
				message.at,
			),
		).toMatchObject({ text: message.text, edited: false });
		expect(
			rc.openEdit(edit.ciphertext, edit.message_id, edit.at),
		).toMatchObject({ text: edit.text, from: edit.from });
		expect(
			rc.openReaction(reaction.ciphertext, reaction.from, reaction.message_id),
		).toBe(reaction.text);
		expect(
			rc.openMessage(image.ciphertext, image.from, image.message_id, image.at),
		).toMatchObject({ text: image.text, fileHash: vectors.file.hash });

		const plain = await rc.decryptFile(
			fromB64(vectors.file.encrypted),
			vectors.file.hash,
		);
		expect(toB64(plain)).toBe(vectors.file.plaintext);
	});

	it("signs rekeys over the same bytes and opens Go's sealed key", async () => {
		const { rekey } = vectors;
		expect(__test.rekeyMessage(vectors.room_id, rekey.epoch, rekey.keys)).toBe(
			vectors.rekey_message,
		);
		expect(await rc.applyRekey(rekey)).toBe(true);
		expect(toB64(rc.currentKey())).toBe(vectors.new_key);
		expect(rc.epoch()).toBe(1);
	});
});

/** A room whose members all know each other's cards (the first owns it). */
function newRoom(...names: string[]): RoomCrypto[] {
	const key = newRoomKey();
	const members = names.map(
		(n) => new RoomCrypto(Identity.create(n), "room-1", names[0], key),
	);
	shareCards(members);
	return members;
}

function shareCards(members: RoomCrypto[]) {
	for (const from of members) {
		const card = from.myCard();
		for (const to of members) {
			to.addCard(from.me.userId, card);
		}
	}
}

describe("signed envelopes", () => {
	it("bind sender, kind and message", () => {
		const [alice, bob] = newRoom("alice", "bob");
		const now = Date.now();
		const msg = alice.sealMessage("hello");
		expect(bob.openMessage(msg, "alice", "m1", now).text).toBe("hello");
		expect(() => bob.openMessage(msg, "bob", "m1", now)).toThrow(
			UnverifiedError,
		);

		const edit = alice.sealEdit("m1", "edited");
		expect(() => bob.openMessage(edit, "alice", "m2", now, now)).toThrow(
			UnverifiedError,
		);
		expect(bob.openEdit(edit, "m1", now)).toMatchObject({
			text: "edited",
			from: "alice",
		});

		const reaction = alice.sealReaction("m1", "👍");
		expect(() => bob.openMessage(reaction, "alice", "m1", now)).toThrow(
			UnverifiedError,
		);
		expect(() => bob.openReaction(reaction, "alice", "m2")).toThrow(
			UnverifiedError,
		);
		expect(bob.openReaction(reaction, "alice", "m1")).toBe("👍");
	});

	it("stop members from writing as each other", () => {
		const [, bob, mallory] = newRoom("alice", "bob", "mallory");
		// Mallory claims Alice wrote her message: same room key, wrong signer.
		const forger = new RoomCrypto(
			Identity.create("alice"),
			"room-1",
			"alice",
			mallory.currentKey(),
		);
		const forged = forger.sealMessage("send me the codes");
		expect(() => bob.openMessage(forged, "alice", "m1", Date.now())).toThrow(
			UnverifiedError,
		);
		expect(bob.readMessage(forged, true, "alice", "m1", Date.now())).toEqual({
			text: "[Unverified message]",
			failed: true,
			fileHash: "",
		});
		// Her card under Alice's name is refused (the API files it under her ID).
		expect(() => bob.addCard("mallory", forger.myCard())).toThrow(
			UnverifiedError,
		);
		// Unsigned v1 envelopes and bare ciphertext are refused.
		const v1 = encryptText(
			'{"v":1,"k":"m","f":"alice","t":"hi"}',
			mallory.currentKey(),
		);
		expect(() => bob.openMessage(v1, "alice", "m1", Date.now())).toThrow(
			UnverifiedError,
		);
		// So is anyone without a card.
		const eve = new RoomCrypto(
			Identity.create("eve"),
			"room-1",
			"alice",
			bob.currentKey(),
		);
		expect(() =>
			bob.openMessage(eve.sealMessage("hi"), "eve", "m1", Date.now()),
		).toThrow(UnknownSenderError);
		// Plaintext from the server, too.
		expect(bob.readMessage("hi", false, "alice", "m1", Date.now()).failed).toBe(
			true,
		);
	});
});

describe("replays", () => {
	it("refuses a message replayed under another ID or long after", () => {
		const [alice, bob] = newRoom("alice", "bob");
		const now = Date.now();
		const msg = alice.sealMessage("transfer 10");
		bob.openMessage(msg, "alice", "m1", now);
		bob.openMessage(msg, "alice", "m1", now); // history after the event
		expect(() => bob.openMessage(msg, "alice", "m2", now)).toThrow(
			ReplayedError,
		);
		// A client that never saw the original gets it much later.
		const stale = new RoomCrypto(
			Identity.create("x"),
			"room-1",
			"alice",
			alice.currentKey(),
		);
		stale.addCard("alice", alice.myCard());
		expect(() =>
			stale.openMessage(msg, "alice", "m3", now + MAX_CLOCK_SKEW_MS + 60_000),
		).toThrow(ReplayedError);
		expect(failedText(new ReplayedError())).toBe("[Replayed message]");
	});

	it("refuses rolling an edit back", async () => {
		const [alice, bob] = newRoom("alice", "bob");
		const first = alice.sealEdit("m1", "v1");
		await new Promise((r) => setTimeout(r, 5));
		const second = alice.sealEdit("m1", "v2");
		expect(bob.openEdit(second, "m1", Date.now()).text).toBe("v2");
		expect(() => bob.openEdit(first, "m1", Date.now())).toThrow(ReplayedError);
	});
});

describe("files", () => {
	it("are bound to their message by hash", async () => {
		const [alice, bob] = newRoom("alice", "bob");
		const { data, hash } = await alice.encryptFile(
			new TextEncoder().encode("cat"),
		);
		const other = await alice.encryptFile(new TextEncoder().encode("dog"));
		expect(new TextDecoder().decode(await bob.decryptFile(data, hash))).toBe(
			"cat",
		);
		await expect(bob.decryptFile(other.data, hash)).rejects.toThrow(
			FileMismatchError,
		);
		await expect(bob.decryptFile(data, "")).rejects.toThrow(FileMismatchError);
		const msg = alice.sealMessage(
			alice.imageContent("https://x/f.enc", "cat.png", hash),
			hash,
		);
		expect(bob.openMessage(msg, "alice", "m1", Date.now()).fileHash).toBe(hash);
	});
});

describe("rekeying", () => {
	it("leaves the departed member out and keeps history readable", async () => {
		const [alice, bob, mallory] = newRoom("alice", "bob", "mallory");
		const before = bob.sealMessage("before");
		alice.markDeparted("mallory");
		const { rekey, newKey, missing } = await alice.newRekey(
			["alice", "bob", "mallory"],
			0,
		);
		expect(missing).toEqual([]);
		expect(rekey.keys?.mallory).toBeUndefined();
		alice.commitKey(newKey, rekey.epoch);

		expect(await bob.applyRekey(rekey)).toBe(true);
		expect(bob.currentKey()).toEqual(newKey);
		expect(await mallory.applyRekey(rekey)).toBe(false);

		const after = alice.sealMessage("after");
		expect(bob.openMessage(after, "alice", "m2", Date.now()).text).toBe(
			"after",
		);
		expect(() =>
			mallory.openMessage(after, "alice", "m2", Date.now()),
		).toThrow();
		expect(alice.openMessage(before, "bob", "m1", Date.now()).text).toBe(
			"before",
		);
		expect(await bob.applyRekey(rekey)).toBe(false); // replayed rekey
	});

	it("accepts only the owner's signature", async () => {
		const [alice, bob] = newRoom("alice", "bob", "mallory");
		const malloryAsOwner = new RoomCrypto(
			Identity.create("mallory"),
			"room-1",
			"mallory",
			bob.currentKey(),
		);
		bob.addCard("mallory", malloryAsOwner.myCard());
		const { rekey: forged } = await malloryAsOwner.newRekey(["bob"], 0);
		await expect(bob.applyRekey(forged)).rejects.toThrow(BadRekeyError);

		const { rekey } = await alice.newRekey(["alice", "bob"], 0);
		await expect(bob.applyRekey({ ...rekey, epoch: 5 })).rejects.toThrow(
			BadRekeyError,
		);
		await expect(bob.newRekey(["alice"], 0)).rejects.toThrow(BadRekeyError);
	});
});

describe("size limits", () => {
	it("keeps the largest message within the API's limit", () => {
		const [me] = newRoom("629bfa91-c2d7-8ec5-b29e-d701d23d339d");
		let n = MAX_SEALED_BYTES - me.sealedSize("", "");
		expect(me.fits("a".repeat(n))).toBe(true);
		expect(me.fits("a".repeat(n + 1))).toBe(false);
		expect(me.sealMessage("a".repeat(n)).length).toBeLessThanOrEqual(
			MAX_CONTENT_LENGTH,
		);
		// UTF-8 bytes, not characters.
		n = Math.floor(n / 3);
		expect(me.fits("漢".repeat(n))).toBe(true);
		expect(me.fits("漢".repeat(n + 1))).toBe(false);
		// Reactions fit the API's 512.
		expect(
			me.sealReaction("0f8f8a0e-3b2c-8d4e-9a1b-2c3d4e5f6a7b", "👨‍👩‍👧‍👦")
				.length,
		).toBeLessThanOrEqual(512);
	});

	it("shortens long image names so the message fits", async () => {
		const [me] = newRoom("629bfa91-c2d7-8ec5-b29e-d701d23d339d");
		const { hash } = await me.encryptFile(new Uint8Array(4));
		const content = me.imageContent(
			"https://example.com/files/abc.enc",
			`${"長".repeat(2000)}.png`,
			hash,
		);
		expect(me.fits(content, "", hash)).toBe(true);
		expect(content.endsWith(".png")).toBe(true);
	});
});
