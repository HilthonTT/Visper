import GlobalStyle from "@/components/styles/global-style";
import { Terminal } from "@/components/terminal";
import { UserProvider } from "@/contexts/user-context";
import { themeContext, useTheme } from "@/hooks/use-theme";
import { createFileRoute } from "@tanstack/react-router";
import { useEffect, useState } from "react";
import { DefaultTheme, ThemeProvider } from "styled-components";
import { useRoomParams } from "@/hooks/use-room-params";

export const Route = createFileRoute("/")({
  component: App,
});

function App() {
  const [roomParams] = useRoomParams();

  const { theme, themeLoaded, setMode } = useTheme();
  const [selectedTheme, setSelectedTheme] = useState(theme);

  useEffect(() => {
    window.addEventListener(
      "keydown",
      (e) => {
        ["ArrowUp", "ArrowDown"].indexOf(e.code) > -1 && e.preventDefault();
      },
      false,
    );
  }, []);

  useEffect(() => {
    setSelectedTheme(theme);
  }, [themeLoaded]);

  // Update meta tag colors when switching themes
  useEffect(() => {
    const themeColor = theme.colors?.body;

    const metaThemeColor = document.querySelector("meta[name='theme-color']");
    const maskIcon = document.querySelector("link[rel='mask-icon']");
    const metaMsTileColor = document.querySelector(
      "meta[name='msapplication-TileColor']",
    );

    metaThemeColor && metaThemeColor.setAttribute("content", themeColor);
    metaMsTileColor && metaMsTileColor.setAttribute("content", themeColor);
    maskIcon && maskIcon.setAttribute("color", themeColor);
  }, [selectedTheme]);

  const themeSwitcher = (switchTheme: DefaultTheme) => {
    setSelectedTheme(switchTheme);
    setMode(switchTheme);
  };

  return (
    <UserProvider>
      <h1 className="sr-only" aria-label="Visper Web">
        Visper Web
      </h1>

      {themeLoaded && (
        <ThemeProvider theme={selectedTheme}>
          <GlobalStyle theme={theme} />
          <themeContext.Provider value={themeSwitcher}>
            <Terminal searchParams={roomParams} />
          </themeContext.Provider>
        </ThemeProvider>
      )}
    </UserProvider>
  );
}
