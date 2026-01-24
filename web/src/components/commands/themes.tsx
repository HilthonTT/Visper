import lodash from "lodash";
import themes from "../styles/themes";
import { useContext, useEffect } from "react";
import { terminalContext } from "../terminal";
import { themeContext } from "@/hooks/use-theme";
import {
  checkThemeSwitch,
  getCurrentCmdArray,
  isArgInvalid,
} from "@/lib/funcs";
import { Usage } from "../usage";
import { Wrapper } from "../styles/output-styled";
import { ThemeSpan, ThemesWrapper } from "../styles/themes-styled";

const myThemes = lodash.keys(themes);

export const Themes = () => {
  const { arg, history, rerender } = useContext(terminalContext);

  const themeSwitcher = useContext(themeContext);
  const currentCommand = getCurrentCmdArray(history);

  const checkArg = () => {
    return isArgInvalid(arg, "set", myThemes) ? <Usage cmd="themes" /> : null;
  };

  useEffect(() => {
    if (checkThemeSwitch(rerender, currentCommand, myThemes)) {
      themeSwitcher?.(themes[currentCommand[2]]);
    }
  }, [arg, rerender, currentCommand]);

  if (arg.length > 0 || arg.length > 2) {
    return checkArg();
  }

  return (
    <Wrapper data-testid="themes">
      <ThemesWrapper>
        {myThemes.map((myTheme) => (
          <ThemeSpan key={myTheme}>{myTheme}</ThemeSpan>
        ))}
      </ThemesWrapper>
      <Usage cmd="themes" marginY />
    </Wrapper>
  );
};
