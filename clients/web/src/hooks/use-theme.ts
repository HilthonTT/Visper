import { createContext, useCallback, useEffect, useState } from "react";
import type { DefaultTheme } from "styled-components";
import { getFromLS, setToLS } from "@/lib/storage";
import themes, { DEFAULT_THEME_NAME, getTheme } from "@/lib/themes";

const THEME_STORAGE_KEY = "visper-theme";

export const useTheme = () => {
	const [theme, setTheme] = useState<DefaultTheme>(themes[DEFAULT_THEME_NAME]);
	const [themeLoaded, setThemeLoaded] = useState(false);

	const setMode = useCallback((mode: DefaultTheme) => {
		setToLS(THEME_STORAGE_KEY, mode.name);
		setTheme(mode);
	}, []);

	useEffect(() => {
		// Stored value may be stale or tampered with: fall back to the default.
		const stored = getTheme(getFromLS(THEME_STORAGE_KEY));
		if (stored) {
			setTheme(stored);
		}
		setThemeLoaded(true);
	}, []);

	return { theme, themeLoaded, setMode };
};

export const themeContext = createContext<
	((switchTheme: DefaultTheme) => void) | null
>(null);
