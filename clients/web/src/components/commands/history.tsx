import { useTerminal } from "@/contexts/terminal-context";
import { Wrapper } from "../styles/output-styled";

export const History = () => {
	const { history, index } = useTerminal();
	// Commands up to (and including) this one, oldest first.
	const currentHistory = history.slice(index).reverse();

	return (
		<Wrapper data-testid="history">
			{currentHistory.map((cmd, i) => (
				// biome-ignore lint/suspicious/noArrayIndexKey: static list, entries can repeat
				<div key={i}>{cmd}</div>
			))}
		</Wrapper>
	);
};
