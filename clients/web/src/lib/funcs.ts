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

const QUOTES = `'"\``;

/**
 * Expand the raw argument string of `echo` like a shell would, word by word:
 * single-quoted text is literal, double-quoted/backticked text keeps its
 * whitespace and has `$VAR` references expanded, and unquoted text is split
 * on whitespace (words are re-joined with a single space). An unmatched quote
 * is kept as a literal character.
 *
 * e.g. `'a' "$USER"   b` -> `a alice b`, `"x   y"` -> `x   y`.
 */
export function expandEchoArgs(
	input: string,
	variables: Record<string, string>,
): string {
	const words: string[] = [];
	let word = "";
	let inWord = false;
	let i = 0;

	while (i < input.length) {
		const ch = input[i];

		if (/\s/.test(ch)) {
			if (inWord) {
				words.push(word);
				word = "";
				inWord = false;
			}
			i++;
			continue;
		}

		inWord = true;

		if (QUOTES.includes(ch)) {
			const end = input.indexOf(ch, i + 1);
			if (end !== -1) {
				const inner = input.slice(i + 1, end);
				word += ch === "'" ? inner : expandVariables(inner, variables);
				i = end + 1;
				continue;
			}
		}

		// Unquoted run (an unmatched quote at `i` is included literally).
		let j = i + 1;
		while (
			j < input.length &&
			!/\s/.test(input[j]) &&
			!QUOTES.includes(input[j])
		) {
			j++;
		}
		word += expandVariables(input.slice(i, j), variables);
		i = j;
	}

	if (inWord) {
		words.push(word);
	}
	return words.join(" ");
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
