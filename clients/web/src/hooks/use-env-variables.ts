import { useMemo } from "react";
import { useUser } from "@/contexts/user-context";

export const useEnvVariables = (): Record<string, string> => {
	const { userId, secureCode, joinCode } = useUser();

	return useMemo(
		() => ({
			USER: userId || "anonymous",
			USERID: userId || "anonymous",
			SECURECODE: secureCode,
			JOINCODE: joinCode,
			HOME: "/home/visper",
			PWD: "/home/visper",
		}),
		[userId, secureCode, joinCode],
	);
};
