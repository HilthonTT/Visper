import type { ReactNode } from "react";
import { useTerminal } from "@/contexts/terminal-context";
import { About } from "./commands/about";
import { Clear } from "./commands/clear";
import { Echo } from "./commands/echo";
import { Env } from "./commands/env";
import { GeneralOutput } from "./commands/general-output";
import { Help } from "./commands/help";
import { History } from "./commands/history";
import { JoinRoom } from "./commands/join-room";
import { SetJoinCode } from "./commands/set-join-code";
import { SetSecureCode } from "./commands/set-secure-code";
import { SetUserId } from "./commands/set-user-id";
import { Themes } from "./commands/themes";
import { Welcome } from "./commands/welcome";
import { OutputContainer, UsageDiv } from "./styles/output-styled";

/** Commands that accept arguments. All others print a usage line if given any. */
const commandsWithArgs: readonly string[] = [
	"themes",
	"echo",
	"setuserid",
	"setsecurecode",
	"setjoincode",
];

const renderers: Record<string, () => ReactNode> = {
	about: () => <About />,
	clear: () => <Clear />,
	echo: () => <Echo />,
	env: () => <Env />,
	help: () => <Help />,
	history: () => <History />,
	joinroom: () => <JoinRoom />,
	pwd: () => <GeneralOutput>/home/visper</GeneralOutput>,
	setjoincode: () => <SetJoinCode />,
	setsecurecode: () => <SetSecureCode />,
	setuserid: () => <SetUserId />,
	themes: () => <Themes />,
	welcome: () => <Welcome />,
	whoami: () => <GeneralOutput>anonymous</GeneralOutput>,
};

interface OutputProps {
	index: number;
	cmd: string;
}

export const Output = ({ index, cmd }: OutputProps) => {
	const { arg } = useTerminal();

	if (!commandsWithArgs.includes(cmd) && arg.length > 0) {
		return <UsageDiv data-testid="usage-output">Usage: {cmd}</UsageDiv>;
	}

	return (
		<OutputContainer data-testid={index === 0 ? "latest-output" : undefined}>
			{renderers[cmd]?.()}
		</OutputContainer>
	);
};
