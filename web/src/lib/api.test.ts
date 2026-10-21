import { describe, expect, it } from "vitest";
import { buildSelfRoomInviteUrl } from "./api";

describe("buildSelfRoomInviteUrl", () => {
	it("targets the self-room-invite endpoint with encoded query params", () => {
		expect(
			buildSelfRoomInviteUrl("http://localhost:8080", "AB 12", "s&c=1"),
		).toBe(
			"http://localhost:8080/api/v1/users/notifications/self-room-invite?join_code=AB+12&secure_code=s%26c%3D1",
		);
	});
});
