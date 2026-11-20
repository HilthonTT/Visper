import type { SavedRoomCrypto } from "./room-keys";

/**
 * Rooms this browser remembers, so a reload can reopen them without an
 * invite. Off unless the user asks for it per room: a remembered room's keys
 * and this browser's identity for it sit in IndexedDB, unencrypted, where
 * anyone with this browser profile, or script the site serves, can read them.
 *
 * Every call fails soft (private mode, blocked site data, SSR): reads return
 * nothing, writes report false.
 */
export type SavedRoom = {
	roomId: string;
	/** Who we are in it: the API's user ID for this browser's cookie. */
	userId: string;
	joinCode: string;
	ownerName: string;
	/** Milliseconds since the epoch. */
	expiresAt: number;
	savedAt: number;
	crypto: SavedRoomCrypto;
};

const DB_NAME = "visper";
const DB_VERSION = 1;
const STORE = "rooms";

function openDb(): Promise<IDBDatabase> {
	return new Promise((resolve, reject) => {
		if (typeof indexedDB === "undefined") {
			reject(new Error("IndexedDB is unavailable"));
			return;
		}
		const req = indexedDB.open(DB_NAME, DB_VERSION);
		req.onupgradeneeded = () => {
			if (!req.result.objectStoreNames.contains(STORE)) {
				req.result.createObjectStore(STORE, { keyPath: "roomId" });
			}
		};
		req.onsuccess = () => resolve(req.result);
		req.onerror = () => reject(req.error);
		req.onblocked = () => reject(new Error("IndexedDB is blocked"));
	});
}

// Calls run one after another, so a save that is still opening the database
// can't land after a later forget and bring the room back.
let queue: Promise<unknown> = Promise.resolve();

function run<T>(
	mode: IDBTransactionMode,
	op: (store: IDBObjectStore) => IDBRequest<T>,
): Promise<T> {
	const next = queue.then(() => runNow(mode, op));
	queue = next.catch(() => {});
	return next;
}

async function runNow<T>(
	mode: IDBTransactionMode,
	op: (store: IDBObjectStore) => IDBRequest<T>,
): Promise<T> {
	const db = await openDb();
	try {
		return await new Promise<T>((resolve, reject) => {
			const tx = db.transaction(STORE, mode);
			const req = op(tx.objectStore(STORE));
			tx.oncomplete = () => resolve(req.result);
			tx.onerror = () => reject(tx.error);
			tx.onabort = () => reject(tx.error);
		});
	} finally {
		db.close();
	}
}

/** The remembered rooms that haven't expired, soonest to expire last. */
export async function listSavedRooms(now = Date.now()): Promise<SavedRoom[]> {
	let all: SavedRoom[];
	try {
		all = (await run("readonly", (s) => s.getAll())) as SavedRoom[];
	} catch {
		return [];
	}
	const live = all.filter((r) => r?.roomId && r.expiresAt > now);
	// Expired rooms are gone on the server too: drop their keys.
	for (const r of all) {
		if (r?.roomId && !live.includes(r)) {
			void forgetRoom(r.roomId);
		}
	}
	return live.sort((a, b) => b.savedAt - a.savedAt);
}

export async function getSavedRoom(roomId: string): Promise<SavedRoom | null> {
	try {
		return ((await run("readonly", (s) => s.get(roomId))) as SavedRoom) ?? null;
	} catch {
		return null;
	}
}

/** Saves (or replaces) a room; false if this browser won't store it. */
export async function saveRoom(room: SavedRoom): Promise<boolean> {
	try {
		await run("readwrite", (s) => s.put(room));
		return true;
	} catch {
		return false;
	}
}

/** Forgets a room and its keys. */
export async function forgetRoom(roomId: string): Promise<void> {
	try {
		await run("readwrite", (s) => s.delete(roomId));
	} catch {
		// Nothing stored, or storage unavailable.
	}
}
