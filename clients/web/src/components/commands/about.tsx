import styled from "styled-components";
import { CommandChip } from "../command-chip";
import { Accent, Strong, Subtle } from "../styles/ui";

export const About = () => (
	<AboutWrapper data-testid="about">
		<p>
			<Strong>Visper</Strong>{" "}
			<Subtle>
				is an anonymous, real-time chat built on Go and WebSockets.
			</Subtle>
		</p>
		<p>
			<Subtle>
				No accounts, no profiles. Join a room with its join code, unlock it with
				the secure code, and talk.
			</Subtle>
		</p>
		<p>
			<Accent>→</Accent>{" "}
			<Subtle>
				Ready? Set your credentials, then run <CommandChip command="joinroom" />
			</Subtle>
		</p>
	</AboutWrapper>
);

const AboutWrapper = styled.div`
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
  max-width: 44rem;
  margin: 0.4rem 0 1rem;
`;
