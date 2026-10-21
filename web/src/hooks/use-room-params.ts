import { parseAsString, useQueryStates } from "nuqs";

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
