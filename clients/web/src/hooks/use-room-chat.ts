import { useCallback, useEffect, useReducer, useRef, useState } from "react";
import { type ApiRoom, chatApi, roomSocketUrl } from "@/lib/chat-api";
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
	encryptedImageContent,
	encryptFile,
	encryptText,
	fitsMessage,
	MAX_UPLOAD_SIZE,
	type RoomKey,
} from "@/lib/room-crypto";

export type Connection = "connecting" | "open" | "reconnecting" | "closed";

/** Least time between two typing frames we send (the server relays one per 2s). */
const TYPING_SEND_INTERVAL_MS = 2000;
const MAX_RECONNECT_DELAY_MS = 15_000;

/**
 * One joined room: its live state, fed by the room WebSocket (and the REST
 * history), and the actions on it. Everything sent is encrypted with `key`
 * first; the server never sees the key or plaintext.
 *
 * `active` is whether the chat is on screen: read receipts are only sent for
 * messages the user can actually see.
 */
export function useRoomChat(room: ApiRoom, key: RoomKey, active: boolean) {
	const [state, dispatch] = useReducer(chatReducer, room, initialChatState);
	const [connection, setConnection] = useState<Connection>("connecting");
	const socket = useRef<WebSocket | null>(null);
	const lastTypingSent = useRef(0);
	const lastReadSent = useRef("");
	const roomId = room.id;

	// Room WebSocket, reconnecting with backoff until the room closes for us.
	const closed = state.closed;
	useEffect(() => {
		if (closed) {
			setConnection("closed");
			return;
		}
		let stopped = false;
		let attempt = 0;
		let timer: number | undefined;

		const connect = () => {
			const ws = new WebSocket(roomSocketUrl(roomId));
			socket.current = ws;
			ws.onopen = () => {
				attempt = 0;
				lastReadSent.current = ""; // re-announce after a reconnect
				setConnection("open");
			};
			ws.onmessage = (event) => {
				let raw: unknown;
				try {
					raw = JSON.parse(String(event.data));
				} catch {
					return;
				}
				const action = parseRoomEvent(raw, key);
				if (action) {
					dispatch(action);
				}
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
			};
		};
		connect();

		return () => {
			stopped = true;
			window.clearTimeout(timer);
			socket.current?.close();
			socket.current = null;
		};
	}, [roomId, key, closed]);

	// History over REST too: the socket replays only the latest messages.
	useEffect(() => {
		const controller = new AbortController();
		chatApi
			.listMessages(roomId)
			.then((res) => {
				if (!controller.signal.aborted) {
					dispatch({
						type: "messages",
						messages: (res.messages ?? []).map((m) => fromApiMessage(m, key)),
					});
				}
			})
			.catch(() => {
				// The socket's history replay covers the recent messages.
			});
		return () => controller.abort();
	}, [roomId, key]);

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

	const send = useCallback(
		async (text: string, replyToId?: string) => {
			assertFits(text);
			const res = await chatApi.send(roomId, {
				content: encryptText(text, key),
				encrypted: true,
				...(replyToId ? { reply_to_id: replyToId } : {}),
			});
			// The socket echoes it too; adding it now avoids a visible delay.
			dispatch({ type: "messages", messages: [fromApiMessage(res, key)] });
			lastTypingSent.current = 0;
		},
		[roomId, key],
	);

	const edit = useCallback(
		async (id: string, text: string) => {
			assertFits(text);
			await chatApi.update(roomId, id, encryptText(text, key), true);
			dispatch({ type: "updated", id, content: text, failed: false });
		},
		[roomId, key],
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
			const res =
				myReaction(message.reactions, state.me) === emoji
					? await chatApi.removeReaction(roomId, message.id)
					: await chatApi.setReaction(
							roomId,
							message.id,
							encryptText(emoji, key),
							true,
						);
			dispatch({
				type: "reactions",
				id: message.id,
				reactions: fromApiMessage(res, key).reactions,
			});
		},
		[roomId, key, state.me],
	);

	const uploadImage = useCallback(
		async (file: File) => {
			if (file.size > MAX_UPLOAD_SIZE - ENCRYPTED_FILE_OVERHEAD) {
				throw new Error("That image is larger than 5 MB.");
			}
			const data = new Uint8Array(await file.arrayBuffer());
			const encrypted = new Uint8Array(encryptFile(data, key));
			const uploaded = await chatApi.upload(roomId, encrypted);
			await send(encryptedImageContent(uploaded.url, file.name));
		},
		[roomId, key, send],
	);

	const leave = useCallback(async () => {
		dispatch({ type: "closed", reason: "You left the room." });
		await chatApi.leave(roomId);
	}, [roomId]);

	return {
		state,
		connection,
		notifyTyping,
		send,
		edit,
		remove,
		react,
		uploadImage,
		leave,
	};
}

function assertFits(text: string) {
	if (!fitsMessage(text)) {
		throw new Error("That message is too long.");
	}
}
