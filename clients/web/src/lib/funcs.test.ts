// biome-ignore-all lint/suspicious/noTemplateCurlyInString: tests the literal ${VAR} syntax
import { describe, expect, it } from "vitest";
import {
	autocomplete,
	expandEchoArgs,
	expandVariables,
	isArgInvalid,
	parseCommand,
} from "./funcs";

const commands = ["echo", "env", "help", "history", "themes", "setdevicecode"];
const themes = ["dark", "light", "ubuntu", "blue-matrix"];

describe("parseCommand", () => {
	it("splits on any whitespace", () => {
		expect(parseCommand("  setdevicecode   abc ")).toEqual([
			"setdevicecode",
			"abc",
		]);
		expect(parseCommand("echo\thi  there")).toEqual(["echo", "hi", "there"]);
	});

	it("returns an empty command for blank input", () => {
		expect(parseCommand("   ")).toEqual([""]);
	});
});

describe("isArgInvalid", () => {
	it("accepts exactly [action, option]", () => {
		expect(isArgInvalid(["set", "dark"], "set", themes)).toBe(false);
	});

	it("rejects wrong action, unknown option or extra args", () => {
		expect(isArgInvalid(["go", "dark"], "set", themes)).toBe(true);
		expect(isArgInvalid(["set", "nope"], "set", themes)).toBe(true);
		expect(isArgInvalid(["set", "dark", "x"], "set", themes)).toBe(true);
		expect(isArgInvalid(["set"], "set", themes)).toBe(true);
	});
});

describe("expandVariables", () => {
	const vars = { USER: "alice", HOME: "/home/visper" };

	it("expands $VAR and ${VAR}", () => {
		expect(expandVariables("hi $USER at ${HOME}!", vars)).toBe(
			"hi alice at /home/visper!",
		);
	});

	it("leaves unknown and prototype names untouched", () => {
		expect(expandVariables("$NOPE ${toString}", vars)).toBe(
			"$NOPE ${toString}",
		);
	});
});

describe("autocomplete", () => {
	it("completes a unique command prefix", () => {
		expect(autocomplete("ech", commands, themes)).toEqual({
			value: "echo",
			hints: [],
		});
	});

	it("lists hints for an ambiguous prefix", () => {
		expect(autocomplete("h", commands, themes)).toEqual({
			hints: ["help", "history"],
		});
	});

	it("does nothing for empty input", () => {
		expect(autocomplete("  ", commands, themes)).toEqual({ hints: [] });
	});

	it("completes the themes subcommand and theme names", () => {
		expect(autocomplete("themes s", commands, themes).value).toBe(
			"themes set ",
		);
		expect(autocomplete("themes set ", commands, themes).hints).toEqual(themes);
		expect(autocomplete("themes set ub", commands, themes).value).toBe(
			"themes set ubuntu",
		);
	});
});

describe("expandEchoArgs", () => {
	const vars = { USER: "alice", HOME: "/home/visper" };

	it("expands unquoted and double-quoted variables", () => {
		expect(expandEchoArgs("hi $USER", vars)).toBe("hi alice");
		expect(expandEchoArgs(`"hi $USER"`, vars)).toBe("hi alice");
		expect(expandEchoArgs("`$HOME`", vars)).toBe("/home/visper");
	});

	it("keeps single-quoted text literal", () => {
		expect(expandEchoArgs("'$USER'", vars)).toBe("$USER");
	});

	it("handles several quoted words independently", () => {
		// Previously treated as one '...'-wrapped string: printed `a' 'b`.
		expect(expandEchoArgs("'a' 'b'", vars)).toBe("a b");
		// Previously printed the quotes and expanded the single-quoted $HOME.
		expect(expandEchoArgs(`"$USER" '$HOME'`, vars)).toBe("alice $HOME");
		expect(expandEchoArgs(`pre"$USER"post`, vars)).toBe("prealicepost");
	});

	it("preserves whitespace inside quotes and collapses it outside", () => {
		expect(expandEchoArgs(`  a   "b   c"   d  `, vars)).toBe("a b   c d");
	});

	it("keeps unmatched quotes literally", () => {
		expect(expandEchoArgs(`it's $USER`, vars)).toBe("it's alice");
		expect(expandEchoArgs(`"open`, vars)).toBe(`"open`);
	});

	it("returns an empty string for no arguments", () => {
		expect(expandEchoArgs("", vars)).toBe("");
		expect(expandEchoArgs("   ", vars)).toBe("");
	});
});
