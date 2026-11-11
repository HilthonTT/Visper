import { afterEach, describe, expect, it, vi } from "vitest";
import { buildSelfRoomInviteUrl, notifySelfRoomInvite } from "./api";

describe("buildSelfRoomInviteUrl", () => {
	it("targets the self-room-invite endpoint with encoded query params", () => {
		expect(
			buildSelfRoomInviteUrl("http://localhost:8080", "AB 12", "s&c=1"),
		).toBe(
			"http://localhost:8080/api/v1/users/notifications/self-room-invite?join_code=AB+12&secure_code=s%26c%3D1",
		);
	});
});

describe("notifySelfRoomInvite", () => {
	const params = { userId: "u", joinCode: "j", secureCode: "s" };

	const mockFetch = (body: string, init: ResponseInit) => {
		vi.stubEnv("VITE_BACKEND_API_URL", "http://api.test/");
		vi.stubGlobal(
			"fetch",
			vi.fn(async () => new Response(body, init)),
		);
	};

	afterEach(() => {
		vi.unstubAllEnvs();
		vi.unstubAllGlobals();
	});

	it("reports the HTTP status when a JSON error body is empty", async () => {
		mockFetch("", {
			status: 502,
			statusText: "Bad Gateway",
			headers: { "content-type": "application/json" },
		});
		await expect(notifySelfRoomInvite(params)).rejects.toThrow(
			"Server returned 502 - Bad Gateway",
		);
	});

	it("uses the API error message", async () => {
		mockFetch(
			JSON.stringify({ error: "room_not_found", message: "bad code" }),
			{
				status: 404,
				headers: { "content-type": "application/json" },
			},
		);
		await expect(notifySelfRoomInvite(params)).rejects.toThrow("bad code");
	});

	it("rejects a non-JSON success body with a clear error", async () => {
		mockFetch("<html></html>", {
			status: 200,
			headers: { "content-type": "text/html" },
		});
		await expect(notifySelfRoomInvite(params)).rejects.toThrow(
			"Unexpected response from server",
		);
	});
});
