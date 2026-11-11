import { memo, useMemo } from "react";
import { TerminalProvider } from "@/contexts/terminal-context";
import { isValidCommand } from "@/lib/commands";
import { parseCommand } from "@/lib/funcs";
import { Output } from "./output";
import {
	CmdNotFound,
	Empty,
	MobileBr,
	MobileSpan,
} from "./styles/terminal-styled";
import { TerminalInfo } from "./terminal-info";

interface HistoryItemProps {
	input: string;
	index: number;
	cmdHistory: string[];
	rerender: boolean;
	clearHistory: () => void;
}

export const HistoryItem = memo(
	({ input, index, cmdHistory, rerender, clearHistory }: HistoryItemProps) => {
		// Memoised so `arg` keeps its identity and command effects don't re-run
		// every time the terminal re-renders.
		const { cmd, arg } = useMemo(() => {
			const [name, ...rest] = parseCommand(input);
			return { cmd: name, arg: rest };
		}, [input]);

		const contextValue = useMemo(
			() => ({ arg, history: cmdHistory, rerender, index, clearHistory }),
			[arg, cmdHistory, rerender, index, clearHistory],
		);

		return (
			<div>
				<div>
					<TerminalInfo />
					<MobileBr />
					<MobileSpan>&#62;</MobileSpan>
					<span data-testid="input-command">{input}</span>
				</div>
				{isValidCommand(cmd) ? (
					<TerminalProvider value={contextValue}>
						<Output index={index} cmd={cmd} />
					</TerminalProvider>
				) : cmd === "" ? (
					<Empty />
				) : (
					<CmdNotFound data-testid={`not-found-${index}`}>
						command not found: {cmd}
					</CmdNotFound>
				)}
			</div>
		);
	},
);

HistoryItem.displayName = "HistoryItem";
