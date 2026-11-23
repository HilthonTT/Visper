import {
	useCallback,
	useEffect,
	useMemo,
	useReducer,
	useRef,
	useState,
} from "react";
import {
	type ApiRoom,
	ChatApiError,
	chatApi,
	roomSocketUrl,
} from "@/lib/chat-api";
import {
	type ChatMessage,
	chatReducer,
	fromApiMessage,
	initialChatState,
	myReaction,
	parseRoomEvent,
} from "@/lib/chat-state";
import {
	ENCRYPTED_FILE_OVERHEAD,
	MAX_UPLOAD_SIZE,
	type RoomKey,
} from "@/lib/room-crypto";
import {
	BadRekeyError,
	Identity,
	type Rekey,
	RoomCrypto,
} from "@/lib/room-keys";
import { forgetRoom, saveRoom } from "@/lib/saved-rooms";

export type Connection = "connecting" | "open" | "reconnecting" | "closed";

/** Least time between two typing frames we send (the server relays one per 2s). */
const TYPING_SEND_INTERVAL_MS = 2000;
const MAX_RECONNECT_DELAY_MS = 15_000;
/** Failed reconnects before checking whether the room still exists for us. */
const PROBE_AFTER_ATTEMPTS = 2;
/** Statuses meaning the room is gone (or we are no longer in it). */
const ROOM_GONE_STATUSES = [403, 404, 410];

const ROTATED_NOTICE =
	"The room key was rotated: old invites no longer work. Open invite for the new one.";
const EXCLUDED_NOTICE =
	"The owner changed the room key without you (they had no key card for you), so anything you send could be read by members who left. Sending is off until you get the new key: ask the owner for a new invite.";
const KEY_CHANGED_NOTICE =
	"A member you verified has a new key. Only their old one is trusted until you check it again in members.";

/**
 * One joined room: its live state, fed by the room WebSocket (and the REST
 * history), and the actions on it. Everything sent is signed and encrypted
 * first (see RoomCrypto); the server never sees a key or plaintext.
 *
 * Before loading anything it fetches the members' key cards (to verify who
 * wrote what) and publishes ours. When a member leaves or is kicked, the
 * owner's browser rotates the room key so they can't read what follows.
 *
 * `active` is whether the chat is on screen: read receipts are only sent for
 * messages the user can actually see.
 *
 * `opts.restored` is a remembered room's crypto state (same identity, keys
 * and owner pin as before the reload); `opts.remember` starts with the room
 * remembered, which saves that state to this browser as it changes.
 */
export function useRoomChat(
	room: ApiRoom,
	key: RoomKey,
	active: boolean,
	opts: { restored?: RoomCrypto; remember?: boolean } = {},
) {
	const [state, dispatch] = useReducer(chatReducer, room, initialChatState);
	const [connection, setConnection] = useState<Connection>("connecting");
	const socket = useRef<WebSocket | null>(null);
	const lastTypingSent = useRef(0);
	const lastReadSent = useRef("");
	const roomId = room.id;
	const me = room.current_user.id;
	const ownerId = room.owner.id;

	// This session's identity and the room's keys. The owner is pinned here
	// (or was, when the remembered room was first entered): only their
	// rekeys are ever accepted.
	const restored = opts.restored;
	const crypto = useMemo(
		() => restored ?? new RoomCrypto(Identity.create(me), roomId, ownerId, key),
		[restored, me, roomId, ownerId, key],
	);
	const [remember, setRemember] = useState(Boolean(opts.remember));
	/** Set when the user asked to remember the room but the browser refused. */
	const [rememberFailed, setRememberFailed] = useState(false);
	/** Set once our key card is published and the others' are loaded. */
	const [ready, setReady] = useState(false);
	/** Bumped when the room key changes, to refresh the invite. */
	const [keyVersion, setKeyVersion] = useState(0);
	/** A note about the room's keys (e.g. that the key was rotated). */
	const [notice, setNotice] = useState("");
	/** Set when a live rekey left us out; sending is off until one includes us. */
	const excluded = useRef(false);
	/** Bumped when a member's key verification changes, to redraw it. */
	const [trustVersion, setTrustVersion] = useState(0);

	const publishCard = useCallback(async () => {
		const card = crypto.myCard();
		crypto.addCard(me, card);
		await chatApi.putCard(roomId, card);
	}, [crypto, me, roomId]);

	/** Loads the members' cards and applies a rekey we missed. */
	const syncCrypto = useCallback(async () => {
		const st = await chatApi.getCrypto(roomId);
		crypto.addCards(st.cards);
		if (await crypto.applyRekey(st.rekey)) {
			await publishCard();
			setKeyVersion((v) => v + 1);
			setNotice(ROTATED_NOTICE);
		}
		return st;
	}, [crypto, roomId, publishCard]);

	// Rotations run one at a time.
	const rotation = useRef<Promise<void>>(Promise.resolve());
	/** Rotates the room key (owner only), leaving departed members out. */
	const rotateKey = useCallback(() => {
		if (!crypto.isOwner()) {
			return;
		}
		rotation.current = rotation.current
			.then(async () => {
				for (let attempt = 0; ; attempt++) {
					const st = await chatApi.getCrypto(roomId);
					crypto.addCards(st.cards);
					const { rekey, newKey, missing } = await crypto.newRekey(
						st.members ?? [],
						st.key_epoch,
					);
					try {
						await chatApi.postRekey(roomId, rekey);
					} catch (e) {
						// Someone else's rekey got in first: retry on top of it.
						if (e instanceof ChatApiError && e.status === 409 && attempt < 1) {
							continue;
						}
						throw e;
					}
					crypto.commitKey(newKey, rekey.epoch);
					await publishCard();
					setKeyVersion((v) => v + 1);
					setNotice(
						missing.length > 0
							? `${ROTATED_NOTICE} ${missing.length} member(s) couldn't get the new key and need a new invite.`
							: ROTATED_NOTICE,
					);
					return;
				}
			})
			.catch(() => {
				setNotice(
					"Couldn't rotate the room key after a member left; they may still read new messages.",
				);
			});
	}, [crypto, roomId, publishCard]);

	// Keys first: publish our card and load everyone else's.
	useEffect(() => {
		let stopped = false;
		setReady(false);
		(async () => {
			try {
				const st = await syncCrypto();
				await publishCard();
				if (st.rekey_needed) {
					rotateKey();
				}
			} catch {
				if (!stopped) {
					setNotice(
						"Couldn't load the room's keys: messages may show as unverified.",
					);
				}
			}
			if (!stopped) {
				setReady(true);
			}
		})();
		return () => {
			stopped = true;
		};
	}, [syncCrypto, publishCard, rotateKey]);

	// Room WebSocket, reconnecting with backoff until the room closes for us.
	const closed = state.closed;
	useEffect(() => {
		if (closed) {
			setConnection("closed");
			return;
		}
		if (!ready) {
			return;
		}
		let stopped = false;
		let attempt = 0;
		let opened = false;
		let timer: number | undefined;
		// Events are handled in order, one at a time: a rekey must be applied
		// before the messages encrypted with its key.
		let queue: Promise<void> = Promise.resolve();

		// After a reconnect, catch up on what the socket missed: its replay
		// covers only the latest messages, and neither deletions, member
		// changes nor key changes.
		const resync = async () => {
			const st = await syncCrypto().catch(() => null);
			// Someone left (or was kicked) while we were disconnected.
			if (st?.rekey_needed) {
				rotateKey();
			}
			chatApi
				.get(roomId)
				.then((res) => {
					if (!stopped) {
						dispatch({ type: "members", members: res.members ?? [] });
					}
				})
				.catch(() => {});
			chatApi
				.listMessages(roomId)
				.then((res) => {
					if (!stopped) {
						dispatch({
							type: "history",
							messages: (res.messages ?? []).map((m) =>
								fromApiMessage(m, crypto),
							),
						});
					}
				})
				.catch(() => {});
		};

		const handle = async (raw: unknown) => {
			const { type, data } = (raw ?? {}) as { type?: unknown; data?: unknown };
			const d = (typeof data === "object" && data !== null ? data : {}) as {
				userId?: unknown;
				card?: unknown;
			};
			if (type === "member.card") {
				if (typeof d.userId === "string" && typeof d.card === "string") {
					const before = crypto.keyStatus(d.userId);
					try {
						crypto.addCard(d.userId, d.card);
					} catch {
						// Forged, or sealed with a key we don't have.
					}
					if (
						before === "verified" &&
						crypto.keyStatus(d.userId) === "changed" &&
						!stopped
					) {
						setTrustVersion((v) => v + 1);
						setNotice(KEY_CHANGED_NOTICE);
					}
				}
				return;
			}
			if (type === "room.rekeyed") {
				try {
					// It arrived live, so we were a member when it was made: a
					// rekey with no key for us left us out (unlike one found on
					// entry, which may simply predate our invite).
					const result = await crypto.applyRekeyResult(data as Rekey);
					if (result === "applied") {
						excluded.current = false;
						await publishCard();
						if (!stopped) {
							setKeyVersion((v) => v + 1);
							setNotice(ROTATED_NOTICE);
						}
					} else if (result === "excluded") {
						excluded.current = true;
						if (!stopped) {
							setNotice(EXCLUDED_NOTICE);
						}
						// So the owner's next rotation can include us.
						await publishCard();
					}
				} catch (e) {
					if (!stopped) {
						setNotice(
							e instanceof BadRekeyError
								? "Ignored a room key change that the owner didn't sign."
								: "Couldn't apply the new room key.",
						);
					}
				}
				return;
			}
			const action = parseRoomEvent(raw, crypto);
			if (!action || stopped) {
				return;
			}
			dispatch(action);
			if (action.type === "joined") {
				// Only members can connect: someone who departed and is back
				// must be included in rekeys again.
				crypto.clearDeparted(action.member.id);
			}
			if (
				(action.type === "left" || action.type === "kicked") &&
				action.userId !== me
			) {
				crypto.markDeparted(action.userId);
				rotateKey();
			}
		};

		const connect = () => {
			const ws = new WebSocket(roomSocketUrl(roomId));
			socket.current = ws;
			ws.onopen = () => {
				attempt = 0;
				lastReadSent.current = ""; // re-announce after a reconnect
				setConnection("open");
				if (opened) {
					queue = queue.then(resync);
				}
				opened = true;
			};
			ws.onmessage = (event) => {
				let raw: unknown;
				try {
					raw = JSON.parse(String(event.data));
				} catch {
					return;
				}
				queue = queue.then(() => handle(raw)).catch(() => {});
			};
			ws.onclose = () => {
				if (socket.current === ws) {
					socket.current = null;
				}
				if (stopped) {
					return;
				}
				setConnection("reconnecting");
				const delay = Math.min(1000 * 2 ** attempt, MAX_RECONNECT_DELAY_MS);
				attempt++;
				timer = window.setTimeout(connect, delay);
				if (attempt >= PROBE_AFTER_ATTEMPTS) {
					// A browser can't see why a WebSocket handshake failed: ask
					// over REST whether the room is still there for us.
					chatApi.listMessages(roomId).catch((e: unknown) => {
						if (
							!stopped &&
							e instanceof ChatApiError &&
							ROOM_GONE_STATUSES.includes(e.status)
						) {
							dispatch({
								type: "closed",
								reason: "This room is no longer available.",
							});
						}
					});
				}
			};
		};
		connect();

		return () => {
			stopped = true;
			window.clearTimeout(timer);
			socket.current?.close();
			socket.current = null;
		};
	}, [roomId, crypto, closed, ready, me, syncCrypto, publishCard, rotateKey]);

	// Rooms expire without a room.deleted event; close the chat on time.
	const expiresAt = state.expiresAt;
	useEffect(() => {
		if (!expiresAt || closed) {
			return;
		}
		const expire = () =>
			dispatch({ type: "closed", reason: "This room has expired." });
		const ms = expiresAt - Date.now();
		if (ms <= 0) {
			expire();
			return;
		}
		// Rooms last at most 7 days, well within setTimeout's ~24.8-day range.
		const id = window.setTimeout(expire, ms);
		return () => window.clearTimeout(id);
	}, [expiresAt, closed]);

	// A remembered room is saved whenever its keys, pins or members change,
	// and forgotten once it closes for us (left, removed, deleted, expired)
	// or the user stops remembering it.
	const members = state.members;
	const joinCode = state.joinCode;
	useEffect(() => {
		if (closed || !remember) {
			void forgetRoom(roomId);
			return;
		}
		// keyVersion, trustVersion and members mark changes inside crypto.
		void [keyVersion, trustVersion, members];
		saveRoom({
			roomId,
			userId: me,
			joinCode,
			ownerName: room.owner.username,
			expiresAt,
			savedAt: Date.now(),
			crypto: crypto.save(),
		}).then((ok) => setRememberFailed(!ok));
	}, [
		closed,
		remember,
		roomId,
		me,
		joinCode,
		room.owner.username,
		expiresAt,
		crypto,
		keyVersion,
		trustVersion,
		members,
	]);

	// History over REST too (once the cards are in): the socket replays only
	// the latest messages.
	useEffect(() => {
		if (!ready) {
			return;
		}
		const controller = new AbortController();
		chatApi
			.listMessages(roomId)
			.then((res) => {
				if (!controller.signal.aborted) {
					dispatch({
						type: "history",
						messages: (res.messages ?? []).map((m) =>
							fromApiMessage(m, crypto),
						),
					});
				}
			})
			.catch(() => {
				// The socket's history replay covers the recent messages.
			});
		return () => controller.abort();
	}, [roomId, crypto, ready]);

	// Typing indicators expire.
	useEffect(() => {
		const id = window.setInterval(
			() => dispatch({ type: "expireTyping", now: Date.now() }),
			1000,
		);
		return () => window.clearInterval(id);
	}, []);

	// Read receipts for the latest message, while it is on screen.
	// (Re)sent once the socket is open, e.g. after a reconnect.
	const latestId = state.messages.at(-1)?.id ?? "";
	const open = connection === "open";
	useEffect(() => {
		const send = () => {
			const ws = socket.current;
			if (
				!open ||
				!latestId ||
				!active ||
				document.visibilityState !== "visible" ||
				ws?.readyState !== WebSocket.OPEN ||
				lastReadSent.current === latestId
			) {
				return;
			}
			ws.send(JSON.stringify({ type: "read", messageId: latestId }));
			lastReadSent.current = latestId;
		};
		send();
		document.addEventListener("visibilitychange", send);
		return () => document.removeEventListener("visibilitychange", send);
	}, [latestId, active, open]);

	const notifyTyping = useCallback(() => {
		const ws = socket.current;
		const now = Date.now();
		if (
			ws?.readyState !== WebSocket.OPEN ||
			now - lastTypingSent.current < TYPING_SEND_INTERVAL_MS
		) {
			return;
		}
		lastTypingSent.current = now;
		ws.send(JSON.stringify({ type: "typing" }));
	}, []);

	const assertReady = useCallback(() => {
		if (!ready) {
			throw new Error("Still loading the room's keys. Try again in a moment.");
		}
		if (excluded.current) {
			throw new Error(EXCLUDED_NOTICE);
		}
	}, [ready]);

	const sendSealed = useCallback(
		async (text: string, replyToId?: string, fileHash = "") => {
			assertReady();
			if (!crypto.fits(text, "", fileHash)) {
				throw new Error("That message is too long.");
			}
			const res = await chatApi.send(roomId, {
				content: crypto.sealMessage(text, fileHash),
				encrypted: true,
				...(replyToId ? { reply_to_id: replyToId } : {}),
			});
			// The socket echoes it too; adding it now avoids a visible delay.
			dispatch({ type: "messages", messages: [fromApiMessage(res, crypto)] });
			lastTypingSent.current = 0;
		},
		[roomId, crypto, assertReady],
	);

	const send = useCallback(
		(text: string, replyToId?: string) => sendSealed(text, replyToId),
		[sendSealed],
	);

	const edit = useCallback(
		async (id: string, text: string) => {
			assertReady();
			if (!crypto.fits(text, id)) {
				throw new Error("That message is too long.");
			}
			await chatApi.update(roomId, id, crypto.sealEdit(id, text), true);
			dispatch({ type: "updated", id, content: text, failed: false });
		},
		[roomId, crypto, assertReady],
	);

	const remove = useCallback(
		async (id: string) => {
			await chatApi.remove(roomId, id);
			dispatch({ type: "deleted", id });
		},
		[roomId],
	);

	/** Sets the user's reaction, or removes it if it is already `emoji`. */
	const react = useCallback(
		async (message: ChatMessage, emoji: string) => {
			assertReady();
			const res =
				myReaction(message.reactions, state.me) === emoji
					? await chatApi.removeReaction(roomId, message.id)
					: await chatApi.setReaction(
							roomId,
							message.id,
							crypto.sealReaction(message.id, emoji),
							true,
						);
			dispatch({
				type: "reactions",
				id: message.id,
				reactions: fromApiMessage(res, crypto).reactions,
			});
		},
		[roomId, crypto, state.me, assertReady],
	);

	const uploadImage = useCallback(
		async (file: File) => {
			assertReady();
			if (file.size > MAX_UPLOAD_SIZE - ENCRYPTED_FILE_OVERHEAD) {
				throw new Error("That image is larger than 5 MB.");
			}
			const plain = new Uint8Array(await file.arrayBuffer());
			const { data, hash } = await crypto.encryptFile(plain);
			const uploaded = await chatApi.upload(roomId, data);
			// The message carries the file's hash, so the server can't swap it.
			await sendSealed(
				crypto.imageContent(uploaded.url, file.name, hash),
				undefined,
				hash,
			);
		},
		[roomId, crypto, sendSealed, assertReady],
	);

	/** Leaves the room; throws (and stays in it) if the server refuses. */
	const leave = useCallback(async () => {
		try {
			await chatApi.leave(roomId);
		} catch (e) {
			// Already gone for us: nothing left to leave.
			if (
				!(e instanceof ChatApiError && ROOM_GONE_STATUSES.includes(e.status))
			) {
				throw e;
			}
		}
		void forgetRoom(roomId);
		dispatch({ type: "closed", reason: "You left the room." });
	}, [roomId]);

	/** Deletes the room for everyone (owner only). */
	const deleteRoom = useCallback(async () => {
		await chatApi.deleteRoom(roomId);
		void forgetRoom(roomId);
		dispatch({ type: "closed", reason: "You deleted the room." });
	}, [roomId]);

	/** Replaces the join code (owner only); the room key stays the same. */
	const newJoinCode = useCallback(async () => {
		await chatApi.newJoinCode(roomId);
		// The socket announces the new code too; this covers a dropped one.
		const res = await chatApi.get(roomId);
		dispatch({ type: "roomUpdated", joinCode: res.join_code });
	}, [roomId]);

	/**
	 * Removes a member (owner only). The room's error.kicked event then
	 * rotates the key; the member is left out of it from now on even if
	 * that event is late.
	 */
	const kick = useCallback(
		async (userId: string) => {
			await chatApi.kick(roomId, userId);
			crypto.markDeparted(userId);
			dispatch({ type: "kicked", userId, reason: "" });
		},
		[roomId, crypto],
	);

	const listFiles = useCallback(
		async () => (await chatApi.listFiles(roomId)) ?? [],
		[roomId],
	);

	const deleteFile = useCallback(
		(fileId: string) => chatApi.deleteFile(roomId, fileId),
		[roomId],
	);

	/** Pins userId's key with the fingerprint the user compared. */
	const verifyMember = useCallback(
		(userId: string, fingerprint: string) => {
			crypto.verifyMember(userId, fingerprint);
			setTrustVersion((v) => v + 1);
		},
		[crypto],
	);

	const unverifyMember = useCallback(
		(userId: string) => {
			crypto.unverifyMember(userId);
			setTrustVersion((v) => v + 1);
		},
		[crypto],
	);

	return {
		state,
		connection,
		crypto,
		/** Changes whenever a member's key status may have changed. */
		trustVersion,
		verifyMember,
		unverifyMember,
		/** The key to put in invites; changes when the room key is rotated. */
		inviteKey: useMemo(
			// keyVersion marks when crypto's current key changed.
			() => (keyVersion >= 0 ? crypto.currentKey() : key),
			[crypto, keyVersion, key],
		),
		ready,
		notice,
		clearNotice: useCallback(() => setNotice(""), []),
		notifyTyping,
		send,
		edit,
		remove,
		react,
		uploadImage,
		leave,
		deleteRoom,
		newJoinCode,
		kick,
		listFiles,
		deleteFile,
		/** Whether this browser keeps the room's keys to reopen it later. */
		remember,
		setRemember,
		rememberFailed,
	};
}
