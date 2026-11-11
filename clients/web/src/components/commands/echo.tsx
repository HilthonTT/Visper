import { Wrapper } from "@/components/styles/output-styled";
import { useTerminal } from "@/contexts/terminal-context";
import { useEnvVariables } from "@/hooks/use-env-variables";
import { expandEchoArgs } from "@/lib/funcs";

export const Echo = () => {
	const { rawArg } = useTerminal();
	const variables = useEnvVariables();

	// Parsed from the raw input (not the whitespace-split `arg`) so quoted
	// text keeps its spacing. Rendered as a text node, so expanded values
	// can't inject markup.
	const output = expandEchoArgs(rawArg, variables);

	return <Wrapper style={{ whiteSpace: "pre-wrap" }}>{output}</Wrapper>;
};
