import styled from "styled-components";
import { CommandChip } from "../command-chip";
import { ChipRow, Kbd, Muted, Panel, SectionLabel, Subtle } from "../styles/ui";

const LOGO = String.raw`
 _    ___
| |  / (_)________  ___  _____
| | / / / ___/ __ \/ _ \/ ___/
| |/ / (__  ) /_/ /  __/ /
|___/_/____/ .___/\___/_/
          /_/`;

const FEATURES = [
	["No sign-up", "pick a name, get a room, start talking"],
	["Encrypted rooms", "messages are sealed with the room's secure code"],
	["Terminal native", "same rooms from this page or the `visper` CLI"],
] as const;

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

		<Features as="ul">
			{FEATURES.map(([title, desc]) => (
				<li key={title}>
					<FeatureTitle>{title}</FeatureTitle>
					<Subtle>{desc}</Subtle>
				</li>
			))}
		</Features>
	</Hero>
);

const Hero = styled.section`
  display: grid;
  grid-template-columns: minmax(0, 1.4fr) minmax(0, 1fr);
  gap: 1.5rem 2.5rem;
  align-items: center;
  margin: 0.5rem 0 1.75rem;

  @media (max-width: 860px) {
    grid-template-columns: minmax(0, 1fr);
  }
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

const Features = styled(Panel)`
  display: flex;
  flex-direction: column;
  gap: 0.9rem;
  margin: 0;
  list-style: none;

  li {
    display: flex;
    flex-direction: column;
    padding-left: 0.9rem;
    border-left: 2px solid ${({ theme }) => theme.colors.primary};
  }

  @media (max-width: 640px) {
    display: none;
  }
`;

const FeatureTitle = styled.span`
  font-weight: 700;
  color: ${({ theme }) => theme.colors.text[100]};
`;
