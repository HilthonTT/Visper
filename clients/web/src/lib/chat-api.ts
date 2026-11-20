import { getBackendUrl } from "./api";

/*
 * REST calls for the browser chat. The browser is identified by the API's
 * HttpOnly `visper_user_id` cookie, a private user token (it can't set the
 * X-User-Token header on the room WebSocket), so every request carries
 * credentials. That needs the API's
 * VISPER_CORS_ALLOWORIGINS to list this site: with "*", browsers refuse to
 * send credentials.
 */

export type ApiUser = { id: string; username: string };

export type ApiRoom = {
	id: string;
	join_code: string;
	owner: ApiUser;
	created_at: string;
	expires_at: string;
	members: ApiUser[] | null;
	current_user: ApiUser;
	qr_code_url: string;
};

export type ApiReaction = {
	user_id: string;
	emoji: string;
	encrypted: boolean;
};

export type ApiMessage = {
	id: string;
	room_id: string;
	user_id: string;
	username: string;
	content: string;
	encrypted: boolean;
	created_at: string;
	/** When the content was last edited, if ever. */
	updated_at?: string;
	reply_to_id?: string;
	reactions?: ApiReaction[] | null;
};

/** A new room key sealed to each member, signed by the owner. */
export type ApiRekey = {
	epoch: number;
	keys: Record<string, string> | null;
	signature: string;
};

/** Key cards and the latest rekey (see room-keys.ts). */
export type ApiCryptoState = {
	key_epoch: number;
	rekey_needed: boolean;
	rekey: ApiRekey | null;
	cards: Record<string, string[]> | null;
	members: string[] | null;
};

export type ApiFile = { id: string; url: string };

/**
 * A file in the room's file list. The server only has ciphertext under a
 * fixed name ("file.enc"): the real name travels in the encrypted message
 * that points to `url`.
 */
export type ApiFileInfo = {
	id: string;
	filename: string;
	size: number;
	url: string;
	createdAt: string;
	uploader: { id: string; username?: string };
};

export class ChatApiError extends Error {
	constructor(
		message: string,
		readonly status: number,
	) {
		super(message);
	}
}

/** Message for a request the browser blocked (CORS) or couldn't send. */
export const NETWORK_ERROR_MESSAGE =
	"Couldn't reach the API with this browser's session. Check that it is running and that VISPER_CORS_ALLOWORIGINS lists this site (credentials don't work with \"*\").";

async function request<T>(
	method: string,
	path: string,
	body?: unknown,
	signal?: AbortSignal,
): Promise<T> {
	const init: RequestInit = { method, credentials: "include", signal };
	if (body instanceof FormData) {
		init.body = body;
	} else if (body !== undefined) {
		init.headers = { "Content-Type": "application/json" };
		init.body = JSON.stringify(body);
	}

	// Outside the try: a missing VITE_BACKEND_API_URL is a setup error with
	// its own message, not a network or CORS failure.
	const url = `${getBackendUrl()}/api/v1/${path}`;
	let response: Response;
	try {
		response = await fetch(url, init);
	} catch (e) {
		if (signal?.aborted) {
			throw e;
		}
		throw new ChatApiError(NETWORK_ERROR_MESSAGE, 0);
	}

	const isJson = response.headers
		.get("content-type")
		?.includes("application/json");
	const data: unknown = isJson ? await response.json().catch(() => null) : null;

	if (!response.ok) {
		const err = (data ?? {}) as { error?: string; message?: string };
		throw new ChatApiError(
			err.message ||
				err.error ||
				`Server returned ${response.status} - ${response.statusText}`,
			response.status,
		);
	}
	return data as T;
}

const room = (id: string) => `rooms/${encodeURIComponent(id)}`;
const message = (roomId: string, id: string) =>
	`${room(roomId)}/messages/${encodeURIComponent(id)}`;

export const chatApi = {
	/** Creates a room owned by this browser's user, which joins it. */
	create: (expiryHours: number) =>
		request<ApiRoom>("POST", "rooms", { expiry_hours: expiryHours }),

	joinByCode: (joinCode: string, username?: string) =>
		request<ApiRoom>("POST", "rooms/join-code", {
			join_code: joinCode,
			...(username ? { username } : {}),
		}),

	joinWithToken: (joinCode: string, secureToken: string, username?: string) =>
		request<ApiRoom>("POST", "rooms/join-code/secure", {
			join_code: joinCode,
			secure_token: secureToken,
			...(username ? { username } : {}),
		}),

	get: (roomId: string) => request<ApiRoom>("GET", room(roomId)),

	leave: (roomId: string) => request<unknown>("POST", `${room(roomId)}/leave`),

	/** Deletes the room for everyone (owner only). */
	deleteRoom: (roomId: string) => request<unknown>("DELETE", room(roomId)),

	/**
	 * Replaces the room's join code (owner only), so the old code stops
	 * letting people join. Members and the room key are unchanged.
	 */
	newJoinCode: (roomId: string) =>
		request<unknown>("PUT", `${room(roomId)}/join-code`),

	/** Removes a member (owner only). The API also replaces the join code. */
	kick: (roomId: string, userId: string) =>
		request<unknown>(
			"POST",
			`${room(roomId)}/membership/${encodeURIComponent(userId)}`,
		),

	/** The room's uploaded (encrypted) files, for members. */
	listFiles: (roomId: string) =>
		request<ApiFileInfo[] | null>("GET", `${room(roomId)}/files`),

	/** Deletes a file (its uploader or the room owner only). */
	deleteFile: (roomId: string, fileId: string) =>
		request<unknown>(
			"DELETE",
			`${room(roomId)}/files/${encodeURIComponent(fileId)}`,
		),

	listMessages: (roomId: string, limit = 100) =>
		request<{ messages: ApiMessage[] | null }>(
			"GET",
			`${room(roomId)}/messages?limit=${limit}`,
		),

	send: (
		roomId: string,
		body: { content: string; encrypted: boolean; reply_to_id?: string },
	) => request<ApiMessage>("POST", `${room(roomId)}/messages`, body),

	update: (roomId: string, id: string, content: string, encrypted: boolean) =>
		request<unknown>("PUT", message(roomId, id), { content, encrypted }),

	remove: (roomId: string, id: string) =>
		request<unknown>("DELETE", message(roomId, id)),

	setReaction: (
		roomId: string,
		id: string,
		emoji: string,
		encrypted: boolean,
	) =>
		request<ApiMessage>("PUT", `${message(roomId, id)}/reaction`, {
			emoji,
			encrypted,
		}),

	removeReaction: (roomId: string, id: string) =>
		request<ApiMessage>("DELETE", `${message(roomId, id)}/reaction`),

	/** The room's key cards and latest rekey. */
	getCrypto: (roomId: string) =>
		request<ApiCryptoState>("GET", `${room(roomId)}/crypto`),

	/** Publishes this browser's key card for the room. */
	putCard: (roomId: string, card: string) =>
		request<unknown>("PUT", `${room(roomId)}/crypto/card`, { card }),

	/** Publishes a rekey (owner only); 409 if the epoch is out of date. */
	postRekey: (roomId: string, rekey: ApiRekey) =>
		request<unknown>("POST", `${room(roomId)}/crypto/rekey`, rekey),

	/** Uploads an already encrypted file under a fixed, meaningless name. */
	upload: (roomId: string, encrypted: Uint8Array<ArrayBuffer>) => {
		const form = new FormData();
		form.append(
			"file",
			new Blob([encrypted], { type: "application/octet-stream" }),
			"file.enc",
		);
		return request<ApiFile>("POST", `${room(roomId)}/files/upload`, form);
	},
};

/** The room's WebSocket URL (ws: or wss: like the API's base URL). */
export function roomSocketUrl(roomId: string): string {
	const base = getBackendUrl().replace(/^http/, "ws");
	return `${base}/api/v1/${room(roomId)}/ws`;
}
