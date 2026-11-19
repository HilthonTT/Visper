import type { ReactNode } from "react";
import styled from "styled-components";
import { CommandChip } from "../command-chip";
import { ChipRow, Kbd, Muted, SectionLabel, Subtle } from "../styles/ui";

const LOGO = String.raw`
 _    ___
| |  / (_)________  ___  _____
| | / / / ___/ __ \/ _ \/ ___/
| |/ / (__  ) /_/ /  __/ /
|___/_/____/ .___/\___/_/
          /_/`;

const FEATURES: readonly [string, ReactNode][] = [
	["No sign-up", "pick a name, get a room, start talking"],
	["Encrypted rooms", "sealed with a key the server never sees"],
	[
		"Terminal native",
		<>
			same rooms here or in the <code>visper</code> CLI
		</>,
	],
];

const QUICK_START = ["help", "about", "themes", "joinroom"] as const;

export const Welcome = () => (
	<Hero data-testid="welcome">
		<Intro>
			<Logo aria-label="Visper">{LOGO}</Logo>
			<Tagline>
				Anonymous, encrypted, real-time chat — straight from your terminal.
			</Tagline>
			<Muted>
				v1.0.0 ·{" "}
				<SourceLink
					href="https://github.com/HilthonTT/Visper"
					target="_blank"
					rel="noopener noreferrer"
				>
					view source ↗
				</SourceLink>
			</Muted>

			<div>
				<SectionLabel>Quick start</SectionLabel>
				<ChipRow>
					{QUICK_START.map((cmd) => (
						<CommandChip key={cmd} command={cmd} />
					))}
				</ChipRow>
			</div>

			<Subtle>
				<Kbd>Tab</Kbd> complete · <Kbd>↑</Kbd> history · <Kbd>Ctrl</Kbd>+
				<Kbd>L</Kbd> clear
			</Subtle>
		</Intro>

		<Features>
			{FEATURES.map(([title, desc]) => (
				<li key={title}>
					<FeatureTitle>{title}</FeatureTitle>
					<FeatureText>{desc}</FeatureText>
				</li>
			))}
		</Features>
	</Hero>
);

const Hero = styled.section`
  display: flex;
  flex-direction: column;
  gap: 1.5rem;
  margin: 0.5rem 0 1.75rem;
`;

const Intro = styled.div`
  display: flex;
  flex-direction: column;
  gap: 1rem;
`;

const Logo = styled.pre`
  width: fit-content;
  font-family: inherit;
  font-weight: 700;
  line-height: 1.2;
  background: linear-gradient(
    100deg,
    ${({ theme }) => theme.colors.primary},
    ${({ theme }) => theme.colors.secondary}
  );
  background-clip: text;
  -webkit-background-clip: text;
  color: transparent;

  @media (max-width: 420px) {
    font-size: 0.7rem;
  }
`;

const Tagline = styled.p`
  max-width: 34rem;
  font-size: 1.05rem;
  color: ${({ theme }) => theme.colors.text[100]};
`;

const SourceLink = styled.a`
  color: ${({ theme }) => theme.colors.secondary};
  text-decoration: none;
  border-bottom: 1px dashed currentColor;

  &:hover {
    border-bottom-style: solid;
  }
`;

// One row of short facts under the intro, in the terminal's own style
// rather than a floating card; the items stack on narrow screens.
const Features = styled.ul`
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(min(100%, 13rem), 1fr));
  gap: 0.75rem 2rem;
  max-width: 52rem;
  margin: 0;
  padding: 1rem 0 0;
  border-top: 1px dashed ${({ theme }) => theme.colors.border};
  list-style: none;

  li {
    display: flex;
    flex-direction: column;
    gap: 0.15rem;
    min-width: 0;
  }
`;

const FeatureTitle = styled.span`
  font-weight: 700;
  color: ${({ theme }) => theme.colors.text[100]};

  &::before {
    content: "▸ ";
    color: ${({ theme }) => theme.colors.primary};
  }
`;

const FeatureText = styled.span`
  padding-left: 1.2ch;
  color: ${({ theme }) => theme.colors.text[200]};
  font-size: 0.9em;

  code {
    color: ${({ theme }) => theme.colors.secondary};
    font-family: inherit;
  }
`;
