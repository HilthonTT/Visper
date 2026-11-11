/**
 * Non-breaking spaces used to align help output columns.
 * Always yields at least two spaces.
 */
export function generateTabs(num = 0): string {
	return "\xA0".repeat(2 + Math.max(0, num));
}

/**
 * Split a raw terminal line into `[command, ...args]`, collapsing runs of
 * whitespace. An empty/blank line yields `[""]`.
 */
export function parseCommand(input: string): string[] {
	return input.trim().split(/\s+/);
}

/**
 * Check whether `arg` is NOT exactly `[action, <one of options>]`.
 */
export function isArgInvalid(
	arg: string[],
	action: string,
	options: string[],
): boolean {
	return arg.length !== 2 || arg[0] !== action || !options.includes(arg[1]);
}

/**
 * Replace `$VAR` and `${VAR}` references with values from `variables`.
 * Unknown variables are left untouched.
 */
export function expandVariables(
	input: string,
	variables: Record<string, string>,
): string {
	return input.replace(
		/\$(?:\{(\w+)\}|(\w+))/g,
		(match, braced?: string, bare?: string) => {
			const name = braced ?? bare ?? "";
			return Object.hasOwn(variables, name) ? variables[name] : match;
		},
	);
}

/**
 * Strip one layer of matching surrounding quotes or backticks.
 */
export function stripQuotes(input: string): string {
	const match = /^(['"`])([\s\S]*)\1$/.exec(input);
	return match ? match[2] : input;
}

export type Completion = {
	/** New input value, or `undefined` to leave the input unchanged. */
	value?: string;
	/** Candidate list to display when the completion is ambiguous. */
	hints: string[];
};

const pick = (
	candidates: string[],
	partial: string,
	build: (match: string) => string,
): Completion => {
	const matches = candidates.filter((c) => c.startsWith(partial));
	if (matches.length === 1) {
		return { value: build(matches[0]), hints: [] };
	}
	return { hints: matches.length > 1 ? matches : [] };
};

/**
 * Tab completion for the terminal input. Pure: callers apply the result.
 */
export function autocomplete(
	input: string,
	commandNames: string[],
	themeNames: string[],
): Completion {
	if (!input.trim()) {
		return { hints: [] };
	}

	const tokens = input.trimStart().split(/\s+/);

	if (tokens.length === 1) {
		return pick(commandNames, tokens[0], (cmd) => cmd);
	}

	if (tokens[0] === "themes") {
		if (tokens.length === 2) {
			return pick(["set"], tokens[1], () => "themes set ");
		}
		if (tokens.length === 3 && tokens[1] === "set") {
			return pick(themeNames, tokens[2], (t) => `themes set ${t}`);
		}
	}

	return { hints: [] };
}
