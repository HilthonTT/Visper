import { parseAsString, useQueryStates } from "nuqs";
import { useEffect } from "react";
import { roomKeyFromHash } from "@/lib/room-key";

export type RoomParams = {
	joinCode: string;
	secureCode: string;
};

export function useRoomParams() {
	return useQueryStates(
		{
			joinCode: parseAsString.withDefault(""),
			secureCode: parseAsString.withDefault(""),
		},
		{
			clearOnDefault: true,
			shallow: true,
		},
	);
}

/**
 * Hands the room key in an invite link's fragment (`#key=…`) to `setRoomKey`,
 * then drops it from the address bar so it doesn't linger in history. The
 * fragment is never sent to a server.
 */
export function useRoomKeyFromHash(setRoomKey: (key: string) => void) {
	useEffect(() => {
		const key = roomKeyFromHash(window.location.hash);
		if (!key) {
			return;
		}
		setRoomKey(key);
		const { pathname, search } = window.location;
		window.history.replaceState(window.history.state, "", pathname + search);
	}, [setRoomKey]);
}
