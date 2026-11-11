import { useContext, useEffect } from "react";
import { useTerminal } from "@/contexts/terminal-context";
import { themeContext } from "@/hooks/use-theme";
import { isArgInvalid } from "@/lib/funcs";
import { getTheme, themeNames } from "@/lib/themes";
import { Wrapper } from "../styles/output-styled";
import { ThemeSpan, ThemesWrapper } from "../styles/themes-styled";
import { Usage } from "../usage";

export const Themes = () => {
	const { arg, rerender, index } = useTerminal();
	const themeSwitcher = useContext(themeContext);

	const invalid = arg.length > 0 && isArgInvalid(arg, "set", themeNames);
	const target = arg.length > 0 && !invalid ? getTheme(arg[1]) : undefined;

	// Only the freshly submitted command switches the theme; older history
	// entries must not re-apply theirs.
	useEffect(() => {
		if (rerender && index === 0 && target) {
			themeSwitcher?.(target);
		}
	}, [rerender, index, target, themeSwitcher]);

	if (arg.length > 0) {
		return invalid ? <Usage cmd="themes" /> : null;
	}

	return (
		<Wrapper data-testid="themes">
			<ThemesWrapper>
				{themeNames.map((name) => (
					<ThemeSpan key={name}>{name}</ThemeSpan>
				))}
			</ThemesWrapper>
			<Usage cmd="themes" marginY />
		</Wrapper>
	);
};
