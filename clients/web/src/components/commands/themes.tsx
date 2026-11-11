import { useContext, useEffect } from "react";
import styled, { type DefaultTheme, useTheme } from "styled-components";
import { useTerminal } from "@/contexts/terminal-context";
import { themeContext } from "@/hooks/use-theme";
import { isArgInvalid } from "@/lib/funcs";
import themes, { getTheme, themeNames } from "@/lib/themes";
import { UsageDiv, Wrapper } from "../styles/output-styled";
import { Chip } from "../styles/ui";

const ThemesUsage = () => (
	<UsageDiv data-testid="themes-invalid-arg">
		Usage: themes set &lt;theme-name&gt; <br />
		eg: themes set ubuntu
	</UsageDiv>
);

export const Themes = () => {
	const { arg, rerender, index, runCommand } = useTerminal();
	const themeSwitcher = useContext(themeContext);
	const active = useTheme();

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
		return invalid ? <ThemesUsage /> : null;
	}

	return (
		<Wrapper data-testid="themes">
			<Grid>
				{themeNames.map((name) => (
					<Swatch
						key={name}
						type="button"
						$preview={themes[name]}
						aria-pressed={active.name === name}
						title={`Run \`themes set ${name}\``}
						onClick={() => runCommand?.(`themes set ${name}`)}
					>
						<Dots aria-hidden="true">
							<i />
							<i />
							<i />
						</Dots>
						{name}
					</Swatch>
				))}
			</Grid>
			<UsageDiv>
				Click a theme, or run <code>themes set &lt;theme-name&gt;</code>
			</UsageDiv>
		</Wrapper>
	);
};

const Grid = styled.div`
  display: flex;
  flex-wrap: wrap;
  gap: 0.6rem;
  margin: 0.4rem 0 0.6rem;
`;

/* Each swatch previews its own theme rather than the active one. */
const Swatch = styled(Chip)<{ $preview: DefaultTheme }>`
  gap: 0.6rem;
  padding: 0.35rem 0.8rem;
  border-color: ${({ $preview }) => $preview.colors.border};
  background: ${({ $preview }) => $preview.colors.surface};
  color: ${({ $preview }) => $preview.colors.text[100]};

  &:hover {
    border-color: ${({ $preview }) => $preview.colors.primary};
    background: ${({ $preview }) => $preview.colors.surfaceAlt};
  }

  &[aria-pressed="true"] {
    border-color: ${({ $preview }) => $preview.colors.primary};
    box-shadow: 0 0 0 1px ${({ $preview }) => $preview.colors.primary};
  }

  i {
    width: 9px;
    height: 9px;
    border-radius: 50%;
  }
  i:nth-child(1) { background: ${({ $preview }) => $preview.colors.primary}; }
  i:nth-child(2) { background: ${({ $preview }) => $preview.colors.secondary}; }
  i:nth-child(3) { background: ${({ $preview }) => $preview.colors.text[200]}; }
`;

const Dots = styled.span`
  display: inline-flex;
  gap: 0.2rem;
`;
