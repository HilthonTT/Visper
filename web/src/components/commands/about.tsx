import { AboutWrapper, Highlight } from "@/components/styles/about-styled";

export const About = () => {
	return (
		<AboutWrapper data-testid="about">
			<p>
				Welcome to <Highlight>Visper</Highlight>!
			</p>
			<p>
				An <Highlight>anonymous real-time chat application</Highlight> built
				with Go and WebSocket technology.
			</p>
			<p>
				Connect instantly, chat freely, and maintain your privacy. <br />
				No registration required - just join and start chatting.
			</p>
		</AboutWrapper>
	);
};
