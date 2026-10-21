import { useEffect } from "react";
import { useTerminal } from "@/contexts/terminal-context";

export const Clear = () => {
	const { clearHistory } = useTerminal();

	useEffect(() => {
		clearHistory?.();
	}, [clearHistory]);

	return null;
};
