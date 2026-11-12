import { useMemo } from "react";
import { useUser } from "@/contexts/user-context";

export const useEnvVariables = (): Record<string, string> => {
	const { deviceCode, secureCode, joinCode, roomKey } = useUser();

	return useMemo(() => {
		// The device code is "<user id>.<public key>".
		const userId = deviceCode.split(".")[0] || "anonymous";
		return {
			USER: userId,
			USERID: userId,
			DEVICECODE: deviceCode,
			SECURECODE: secureCode,
			JOINCODE: joinCode,
			// The key unlocks the room: never print it.
			ROOMKEY: roomKey ? "(set, hidden)" : "",
			HOME: "/home/visper",
			PWD: "/home/visper",
		};
	}, [deviceCode, secureCode, joinCode, roomKey]);
};
