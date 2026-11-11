import type { DefaultTheme } from "styled-components";

export type Themes = {
	[key: string]: DefaultTheme;
};

const themes: Themes = {
	visper: {
		id: "T_000",
		name: "visper",
		colors: {
			body: "#15131f",
			surface: "#1a1827",
			surfaceAlt: "#221f33",
			border: "#34314a",
			scrollHandle: "#34314a",
			scrollHandleHover: "#46425f",
			primary: "#a78bfa",
			onPrimary: "#13111c",
			secondary: "#f472b6",
			success: "#34d399",
			warning: "#fbbf24",
			danger: "#f87171",
			text: {
				100: "#f1f5f9",
				200: "#94a3b8",
				300: "#64748b",
			},
		},
	},
	dark: {
		id: "T_001",
		name: "dark",
		colors: {
			body: "#080b10",
			surface: "#0f141b",
			surfaceAlt: "#161d27",
			border: "#232c38",
			scrollHandle: "#263040",
			scrollHandleHover: "#33404f",
			primary: "#34d399",
			onPrimary: "#06281d",
			secondary: "#f5a524",
			success: "#4ade80",
			warning: "#fbbf24",
			danger: "#f87171",
			text: {
				100: "#e6edf3",
				200: "#9aa7b4",
				300: "#5d6875",
			},
		},
	},
	light: {
		id: "T_002",
		name: "light",
		colors: {
			body: "#e9edf2",
			surface: "#ffffff",
			surfaceAlt: "#f4f6f9",
			border: "#dbe1e8",
			scrollHandle: "#cfd6de",
			scrollHandleHover: "#b6bfc9",
			primary: "#047857",
			onPrimary: "#ffffff",
			secondary: "#b45309",
			success: "#15803d",
			warning: "#a16207",
			danger: "#dc2626",
			text: {
				100: "#1f2933",
				200: "#52606d",
				300: "#8a96a3",
			},
		},
	},
	"blue-matrix": {
		id: "T_003",
		name: "blue-matrix",
		colors: {
			body: "#030508",
			surface: "#0a0d12",
			surfaceAlt: "#10151c",
			border: "#1a2430",
			scrollHandle: "#1f2b38",
			scrollHandleHover: "#2c3b4b",
			primary: "#00ff9c",
			onPrimary: "#00160d",
			secondary: "#60fdff",
			success: "#00ff9c",
			warning: "#ffe066",
			danger: "#ff5c7a",
			text: {
				100: "#e8fff5",
				200: "#9fc7b7",
				300: "#4c6b60",
			},
		},
	},
	espresso: {
		id: "T_004",
		name: "espresso",
		colors: {
			body: "#1b1917",
			surface: "#262321",
			surfaceAlt: "#2f2b28",
			border: "#3d3834",
			scrollHandle: "#4a443f",
			scrollHandleHover: "#5b544e",
			primary: "#e1e48b",
			onPrimary: "#262321",
			secondary: "#a5c260",
			success: "#a5c260",
			warning: "#e8b85c",
			danger: "#e07a5f",
			text: {
				100: "#f4efe8",
				200: "#c7bfb4",
				300: "#7d756b",
			},
		},
	},
	"green-goblin": {
		id: "T_005",
		name: "green-goblin",
		colors: {
			body: "#000000",
			surface: "#040704",
			surfaceAlt: "#0a110a",
			border: "#163016",
			scrollHandle: "#1d3a1d",
			scrollHandleHover: "#2a522a",
			primary: "#e5e500",
			onPrimary: "#000000",
			secondary: "#04d100",
			success: "#01ff00",
			warning: "#e5e500",
			danger: "#ff3b3b",
			text: {
				100: "#01ff00",
				200: "#04a5b2",
				300: "#2f6b2f",
			},
		},
	},
	ubuntu: {
		id: "T_006",
		name: "ubuntu",
		colors: {
			body: "#22001a",
			surface: "#300a24",
			surfaceAlt: "#3b1030",
			border: "#4f1d41",
			scrollHandle: "#f47845",
			scrollHandleHover: "#e65f31",
			primary: "#87d441",
			onPrimary: "#1b0716",
			secondary: "#f47845",
			success: "#87d441",
			warning: "#f4c145",
			danger: "#ff6b6b",
			text: {
				100: "#ffffff",
				200: "#e1d5dc",
				300: "#a68a9c",
			},
		},
	},
};

export default themes;

export const themeNames = Object.keys(themes);

/** Matches the CLI's default palette. */
export const DEFAULT_THEME_NAME = "visper";

/** Look up a theme by name, returning `undefined` for unknown names. */
export function getTheme(
	name: string | null | undefined,
): DefaultTheme | undefined {
	return name && Object.hasOwn(themes, name) ? themes[name] : undefined;
}
