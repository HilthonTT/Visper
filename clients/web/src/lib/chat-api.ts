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
	reply_to_id?: string;
	reactions?: ApiReaction[] | null;
};

export type ApiFile = { id: string; url: string };

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

	let response: Response;
	try {
		response = await fetch(`${getBackendUrl()}/api/v1/${path}`, init);
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
