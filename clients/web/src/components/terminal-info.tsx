import {
	Prompt,
	PromptArrow,
	PromptHost,
	PromptPath,
	PromptUser,
} from "./styles/terminal-styled";

export const TerminalInfo = () => (
	<Prompt>
		<PromptUser>anonymous</PromptUser>@<PromptHost>visper</PromptHost>{" "}
		<PromptPath>~</PromptPath> <PromptArrow>❯</PromptArrow>
	</Prompt>
);
