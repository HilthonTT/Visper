import themes from "@/components/styles/themes";
import { createContext, useEffect, useState } from "react";
import { setToLS, getFromLS } from "@/lib/storage";
import type { DefaultTheme } from "styled-components";

export const useTheme = () => {
  const [theme, setTheme] = useState<DefaultTheme>(themes.dark);
  const [themeLoaded, setThemeLoaded] = useState(false);

  const setMode = (mode: DefaultTheme) => {
    setToLS("tsn-theme", mode.name);
    setTheme(mode);
  };

  useEffect(() => {
    const localThemeName = getFromLS("tsn-theme");
    localThemeName ? setTheme(themes[localThemeName]) : setTheme(themes.dark);
    setThemeLoaded(true);
  }, []);

  return { theme, themeLoaded, setMode };
};

export const themeContext = createContext<
  ((switchTheme: DefaultTheme) => void) | null
>(null);
