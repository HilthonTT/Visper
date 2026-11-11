import { memo, useMemo } from "react";
import { TerminalProvider } from "@/contexts/terminal-context";
import { isValidCommand } from "@/lib/commands";
import { parseCommand } from "@/lib/funcs";
import { Output } from "./output";
import { CmdNotFound, Empty, InputLine } from "./styles/terminal-styled";
import { Muted } from "./styles/ui";
import { TerminalInfo } from "./terminal-info";

interface HistoryItemProps {
	input: string;
	index: number;
	cmdHistory: string[];
	rerender: boolean;
	clearHistory: () => void;
	runCommand: (input: string) => void;
}

export const HistoryItem = memo(
	({
		input,
		index,
		cmdHistory,
		rerender,
		clearHistory,
		runCommand,
	}: HistoryItemProps) => {
		// Memoised so `arg` keeps its identity and command effects don't re-run
		// every time the terminal re-renders.
		const { cmd, arg, rawArg } = useMemo(() => {
			const [name, ...rest] = parseCommand(input);
			const raw = input.trimStart().slice(name.length);
			return { cmd: name, arg: rest, rawArg: raw };
		}, [input]);

		const contextValue = useMemo(
			() => ({
				arg,
				rawArg,
				history: cmdHistory,
				rerender,
				index,
				clearHistory,
				runCommand,
			}),
			[arg, rawArg, cmdHistory, rerender, index, clearHistory, runCommand],
		);

		return (
			<div>
				<InputLine>
					<TerminalInfo />
					<span data-testid="input-command">{input}</span>
				</InputLine>
				{isValidCommand(cmd) ? (
					<TerminalProvider value={contextValue}>
						<Output index={index} cmd={cmd} />
					</TerminalProvider>
				) : cmd === "" ? (
					<Empty />
				) : (
					<CmdNotFound data-testid={`not-found-${index}`}>
						command not found: {cmd}{" "}
						<Muted>— type `help` for a list of commands</Muted>
					</CmdNotFound>
				)}
			</div>
		);
	},
);

HistoryItem.displayName = "HistoryItem";
