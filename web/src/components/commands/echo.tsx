import { Wrapper } from "@/components/styles/output-styled";
import { useTerminal } from "@/contexts/terminal-context";
import { useEnvVariables } from "@/hooks/use-env-variables";
import { expandVariables, stripQuotes } from "@/lib/funcs";

export const Echo = () => {
	const { arg } = useTerminal();
	const variables = useEnvVariables();

	// Rendered as a text node, so expanded values can't inject markup.
	const output = expandVariables(stripQuotes(arg.join(" ")), variables);

	return <Wrapper>{output}</Wrapper>;
};
