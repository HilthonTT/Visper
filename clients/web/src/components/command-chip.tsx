import type { ReactNode } from "react";
import { useTerminal } from "@/contexts/terminal-context";
import { Chip } from "./styles/ui";

interface CommandChipProps {
	/** Command line submitted when the chip is pressed. */
	command: string;
	children?: ReactNode;
}

/** A command rendered as a button that runs it, as if typed. */
export const CommandChip = ({ command, children }: CommandChipProps) => {
	const { runCommand } = useTerminal();

	return (
		<Chip
			type="button"
			title={`Run \`${command}\``}
			onClick={() => runCommand?.(command)}
		>
			{children ?? command}
		</Chip>
	);
};
