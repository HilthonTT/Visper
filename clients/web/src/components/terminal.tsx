import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useShell } from "@/contexts/shell-context";
import { useUser } from "@/contexts/user-context";
import type { RoomParams } from "@/hooks/use-room-params";
import { commandNames, isValidCommand } from "@/lib/commands";
import { moodForCommand } from "@/lib/companions";
import { autocomplete, parseCommand } from "@/lib/funcs";
import { themeNames } from "@/lib/themes";
import { HistoryItem } from "./history-item";
import { Filler, Form, Hints, Input, Wrapper } from "./styles/terminal-styled";
import { Muted } from "./styles/ui";
import { TerminalInfo } from "./terminal-info";

type HistoryEntry = {
	/** Stable key so past entries don't remount (and re-run effects) on submit. */
	id: number;
	input: string;
};

interface TerminalProps {
	searchParams: RoomParams;
	/** False while another page is shown; the terminal stays mounted. */
	active: boolean;
}

export const Terminal = ({ searchParams, active }: TerminalProps) => {
	const inputRef = useRef<HTMLInputElement>(null);
	const nextId = useRef(1);

	const { setJoinCode, setSecureCode } = useUser();
	const { setMood } = useShell();

	const [inputVal, setInputVal] = useState("");
	const [entries, setEntries] = useState<HistoryEntry[]>([
		{ id: 0, input: "welcome" },
	]);
	const [rerender, setRerender] = useState(false);
	const [hints, setHints] = useState<string[]>([]);
	const [pointer, setPointer] = useState(-1);

	const cmdHistory = useMemo(() => entries.map((e) => e.input), [entries]);

	const handleChange = useCallback((e: React.ChangeEvent<HTMLInputElement>) => {
		setRerender(false);
		setInputVal(e.target.value);
		// Completion candidates belong to the previous input; drop them.
		setHints([]);
	}, []);

	const runCommand = useCallback(
		(input: string) => {
			const entry = { id: nextId.current++, input };
			const [cmd] = parseCommand(input);
			setMood(moodForCommand(cmd, isValidCommand(cmd), entry.id));
			setEntries((prev) => [entry, ...prev]);
			setInputVal("");
			setRerender(true);
			setHints([]);
			setPointer(-1);
			inputRef.current?.focus();
		},
		[setMood],
	);

	const handleSubmit = (e: React.FormEvent<HTMLFormElement>) => {
		e.preventDefault();
		runCommand(inputVal);
	};

	const clearHistory = useCallback(() => {
		setEntries([]);
		setHints([]);
		setPointer(-1);
	}, []);

	const handleKeyDown = (e: React.KeyboardEvent<HTMLInputElement>) => {
		setRerender(false);

		const key = e.key.toLowerCase();
		const ctrlI = e.ctrlKey && key === "i";
		const ctrlL = e.ctrlKey && key === "l";

		if (e.key === "Tab" || ctrlI) {
			e.preventDefault();
			const { value, hints: nextHints } = autocomplete(
				inputVal,
				commandNames,
				themeNames,
			);
			if (value !== undefined) {
				setInputVal(value);
			}
			setHints(nextHints);
			return;
		}

		if (ctrlL) {
			// Prevent the browser from focusing the address bar.
			e.preventDefault();
			clearHistory();
			return;
		}

		if (e.key === "ArrowUp") {
			// Prevent the caret from jumping to the start of the input.
			e.preventDefault();
			if (pointer + 1 >= cmdHistory.length) {
				return;
			}
			setInputVal(cmdHistory[pointer + 1]);
			setPointer(pointer + 1);
			return;
		}

		if (e.key === "ArrowDown") {
			e.preventDefault();
			if (pointer < 0) {
				return;
			}
			if (pointer === 0) {
				setInputVal("");
				setPointer(-1);
				return;
			}
			setInputVal(cmdHistory[pointer - 1]);
			setPointer(pointer - 1);
		}
	};

	// Clicking anywhere focuses the input, unless the user is selecting text
	// or another page is showing.
	useEffect(() => {
		if (!active) {
			return;
		}
		inputRef.current?.focus();
		const focusInput = () => {
			if (window.getSelection()?.toString()) {
				return;
			}
			inputRef.current?.focus();
		};
		document.addEventListener("click", focusInput);
		return () => document.removeEventListener("click", focusInput);
	}, [active]);

	useEffect(() => {
		if (searchParams.joinCode) {
			setJoinCode(searchParams.joinCode);
		}
		if (searchParams.secureCode) {
			setSecureCode(searchParams.secureCode);
		}
	}, [
		searchParams.joinCode,
		searchParams.secureCode,
		setJoinCode,
		setSecureCode,
	]);

	return (
		<Wrapper data-testid="terminal-wrapper">
			<Filler />
			{hints.length > 1 && (
				<Hints>
					{hints.map((hCmd) => (
						<Muted key={hCmd}>{hCmd}</Muted>
					))}
				</Hints>
			)}
			<Form onSubmit={handleSubmit}>
				<label htmlFor="terminal-input">
					<TerminalInfo />
				</label>
				<Input
					aria-label="Command input"
					placeholder={
						entries.length <= 1 ? "try help, or press Tab" : undefined
					}
					type="text"
					id="terminal-input"
					autoComplete="off"
					spellCheck="false"
					autoFocus
					autoCapitalize="off"
					ref={inputRef}
					value={inputVal}
					onKeyDown={handleKeyDown}
					onChange={handleChange}
				/>
			</Form>

			{entries.map(({ id, input }, index) => (
				<HistoryItem
					key={id}
					input={input}
					index={index}
					cmdHistory={cmdHistory}
					rerender={rerender}
					clearHistory={clearHistory}
					runCommand={runCommand}
				/>
			))}
		</Wrapper>
	);
};
