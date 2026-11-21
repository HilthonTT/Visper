import { useRouter } from "@tanstack/react-router";
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
	const { history } = useRouter();
	useEffect(() => {
		const key = roomKeyFromHash(window.location.hash);
		if (!key) {
			return;
		}
		setRoomKey(key);
		// Through the router's history, not window.history: the router would
		// otherwise keep the old location, fragment and all, and put it back
		// in the address bar on its next navigation (nuqs keeps the hash).
		const { pathname, search } = window.location;
		history.replace(pathname + search, window.history.state);
	}, [setRoomKey, history]);
}
