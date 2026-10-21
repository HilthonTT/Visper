import { describe, expect, it } from "vitest";
import themes, { getTheme, themeNames } from "./themes";

describe("getTheme", () => {
	it("returns known themes", () => {
		expect(getTheme("ubuntu")).toBe(themes.ubuntu);
		expect(themeNames).toContain("dark");
	});

	it("returns undefined for unknown or inherited names", () => {
		expect(getTheme("nope")).toBeUndefined();
		expect(getTheme("constructor")).toBeUndefined();
		expect(getTheme(null)).toBeUndefined();
	});
});
