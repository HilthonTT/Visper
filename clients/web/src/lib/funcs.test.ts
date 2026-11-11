// biome-ignore-all lint/suspicious/noTemplateCurlyInString: tests the literal ${VAR} syntax
import { describe, expect, it } from "vitest";
import {
	autocomplete,
	expandVariables,
	generateTabs,
	isArgInvalid,
	parseCommand,
	stripQuotes,
} from "./funcs";

const commands = ["echo", "env", "help", "history", "themes", "setuserid"];
const themes = ["dark", "light", "ubuntu", "blue-matrix"];

describe("parseCommand", () => {
	it("splits on any whitespace", () => {
		expect(parseCommand("  setuserid   abc ")).toEqual(["setuserid", "abc"]);
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

describe("stripQuotes", () => {
	it("strips one layer of matching quotes", () => {
		expect(stripQuotes(`"hello"`)).toBe("hello");
		expect(stripQuotes("'a b'")).toBe("a b");
		expect(stripQuotes("`x`")).toBe("x");
	});

	it("keeps unbalanced quotes", () => {
		expect(stripQuotes(`"hello'`)).toBe(`"hello'`);
	});
});

describe("generateTabs", () => {
	it("pads with non-breaking spaces", () => {
		expect(generateTabs()).toBe("\xA0\xA0");
		expect(generateTabs(3)).toHaveLength(5);
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
