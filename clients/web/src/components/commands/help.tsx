import styled from "styled-components";
import { type Command, commands } from "@/lib/commands";
import { CommandChip } from "../command-chip";
import { Kbd, SectionLabel, Subtle } from "../styles/ui";

const groups: [string, Command[]][] = [
	["General", commands.filter((c) => !c.special)],
	["Rooms", commands.filter((c) => c.special)],
];

const shortcuts = [
	[["Tab"], "autocomplete a command or theme"],
	[["Ctrl", "I"], "same as Tab"],
	[["↑", "↓"], "walk through previous commands"],
	[["Ctrl", "L"], "clear the terminal"],
] as const;

export const Help = () => (
	<HelpWrapper data-testid="help">
		{groups.map(([label, cmds]) => (
			<section key={label}>
				<SectionLabel>{label}</SectionLabel>
				<Grid>
					{cmds.map(({ cmd, desc }) => (
						<Row key={cmd}>
							<CommandChip command={cmd} />
							<Subtle>{desc}</Subtle>
						</Row>
					))}
				</Grid>
			</section>
		))}

		<Shortcuts>
			<SectionLabel>Shortcuts</SectionLabel>
			<Grid>
				{shortcuts.map(([keys, desc]) => (
					<Row key={desc}>
						<Keys>
							{keys.map((k) => (
								<Kbd key={k}>{k}</Kbd>
							))}
						</Keys>
						<Subtle>{desc}</Subtle>
					</Row>
				))}
			</Grid>
		</Shortcuts>
	</HelpWrapper>
);

const HelpWrapper = styled.div`
  display: flex;
  flex-direction: column;
  gap: 1.1rem;
  margin: 0.4rem 0 1rem;
`;

const Grid = styled.div`
  display: grid;
  grid-template-columns: 9.5rem 1fr;
  gap: 0.35rem 1.25rem;
  align-items: center;
  justify-items: start;

  @media (max-width: 520px) {
    grid-template-columns: 1fr;
    gap: 0.2rem;
  }
`;

/* Children sit directly in the parent grid. */
const Row = styled.div`
  display: contents;

  @media (max-width: 520px) {
    & > :last-child {
      margin-bottom: 0.5rem;
    }
  }
`;

const Keys = styled.span`
  display: inline-flex;
  gap: 0.3rem;
`;

const Shortcuts = styled.section`
  @media (hover: none) {
    display: none;
  }
`;
