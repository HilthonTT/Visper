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

export async function notifySelfRoomInvite(
	params: { userId: string; joinCode: string; secureCode: string },
	signal?: AbortSignal,
): Promise<SelfRoomInviteResponse> {
	const response = await fetch(
		buildSelfRoomInviteUrl(getBackendUrl(), params.joinCode, params.secureCode),
		{
			method: "POST",
			headers: { "Content-Type": "application/json" },
			body: JSON.stringify({ user_id: params.userId }),
			signal,
		},
	);

	const isJson = response.headers
		.get("content-type")
		?.includes("application/json");

	if (!response.ok) {
		if (isJson) {
			const data = (await response.json()) as ApiError;
			throw new Error(data.message || "Failed to send notification");
		}
		throw new Error(
			`Server returned ${response.status} - ${response.statusText}`,
		);
	}

	return (await response.json()) as SelfRoomInviteResponse;
}
