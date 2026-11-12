import { parseDeviceCode, parseRoomKey, sealRoomKey } from "./room-key";

/** Base URL of the Visper API, from `VITE_BACKEND_API_URL` (no trailing slash). */
export function getBackendUrl(): string {
	const url = import.meta.env.VITE_BACKEND_API_URL;
	if (!url) {
		throw new Error("VITE_BACKEND_API_URL is not configured");
	}
	return url.replace(/\/+$/, "");
}

export function buildSelfRoomInviteUrl(
	backendUrl: string,
	joinCode: string,
	secureCode: string,
): string {
	const params = new URLSearchParams({
		join_code: joinCode,
		secure_code: secureCode,
	});
	return `${backendUrl}/api/v1/users/notifications/self-room-invite?${params}`;
}

/** Response of `POST /api/v1/users/notifications/self-room-invite`. */
export type SelfRoomInviteResponse =
	| {
			message: string;
			room_id: string;
			already_joined: true;
	  }
	| {
			message: string;
			room_id: string;
			user_id: string;
			username: string;
			notification_sent: true;
			already_joined?: undefined;
	  };

type ApiError = { error?: string; message?: string };

export type SelfRoomInviteParams = {
	/** The CLI's device code: its user id and the public key to seal to. */
	deviceCode: string;
	joinCode: string;
	secureCode: string;
	/** The room key from the invite link; only its sealed form is sent. */
	roomKey: string;
};

/**
 * Sends a room invite to your CLI. The room key is sealed to the CLI's device
 * key first, so the API relays it without being able to read it. Invalid
 * codes throw a RoomKeyError before anything is sent.
 */
export async function notifySelfRoomInvite(
	params: SelfRoomInviteParams,
	signal?: AbortSignal,
): Promise<SelfRoomInviteResponse> {
	const device = parseDeviceCode(params.deviceCode);
	const sealedKey = await sealRoomKey(
		parseRoomKey(params.roomKey),
		device.publicKey,
	);

	const response = await fetch(
		buildSelfRoomInviteUrl(getBackendUrl(), params.joinCode, params.secureCode),
		{
			method: "POST",
			headers: { "Content-Type": "application/json" },
			body: JSON.stringify({ user_id: device.userId, sealed_key: sealedKey }),
			signal,
		},
	);

	// A JSON content-type does not guarantee a parseable body (empty 502 from a
	// proxy, truncated response): fall back to `null` instead of surfacing a
	// cryptic SyntaxError that hides the HTTP status.
	const isJson = response.headers
		.get("content-type")
		?.includes("application/json");
	const data: unknown = isJson ? await response.json().catch(() => null) : null;

	if (!response.ok) {
		const apiError = (data ?? {}) as ApiError;
		throw new Error(
			apiError.message ||
				apiError.error ||
				`Server returned ${response.status} - ${response.statusText}`,
		);
	}

	if (data === null || typeof data !== "object") {
		throw new Error("Unexpected response from server");
	}
	return data as SelfRoomInviteResponse;
}

/** True when the API answers `GET /health` with a 2xx; false on any failure. */
export async function checkHealth(signal?: AbortSignal): Promise<boolean> {
	try {
		const response = await fetch(`${getBackendUrl()}/health`, { signal });
		return response.ok;
	} catch {
		return false;
	}
}
