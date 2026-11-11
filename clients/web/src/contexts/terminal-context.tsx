import { createContext, type ReactNode, useContext } from "react";

export type TerminalContextType = {
	/** Arguments of this history entry's command (command name excluded). */
	arg: string[];
	/** Full command history, newest first. */
	history: string[];
	/** True right after a submit, false while the user is typing. */
	rerender: boolean;
	/** Position of this entry in `history` (0 = latest). */
	index: number;
	clearHistory?: () => void;
};

const TerminalContext = createContext<TerminalContextType | undefined>(
	undefined,
);

interface TerminalProviderProps {
	value: TerminalContextType;
	children: ReactNode;
}

export const TerminalProvider = ({
	value,
	children,
}: TerminalProviderProps) => (
	<TerminalContext.Provider value={value}>{children}</TerminalContext.Provider>
);

export const useTerminal = () => {
	const context = useContext(TerminalContext);
	if (!context) {
		throw new Error("useTerminal must be used within TerminalProvider");
	}
	return context;
};
