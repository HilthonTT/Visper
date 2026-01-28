import { parseAsString, useQueryStates } from "nuqs";

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
