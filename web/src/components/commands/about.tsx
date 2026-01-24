import {
  AboutWrapper,
  HighlightAlt,
  HighlightSpan,
} from "@/components/styles/about-styled";

export const About = () => {
  return (
    <AboutWrapper data-testid="about">
      <p>
        Welcome to <HighlightSpan>Visper</HighlightSpan>!
      </p>
      <p>
        An <HighlightAlt>anonymous real-time chat application</HighlightAlt>{" "}
        built with Go and WebSocket technology.
      </p>
      <p>
        Connect instantly, chat freely, and maintain your privacy. <br />
        No registration required - just join and start chatting.
      </p>
    </AboutWrapper>
  );
};
