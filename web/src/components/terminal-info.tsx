import { User, WebsiteName, Wrapper } from "./styles/terminal-info-styled";

export const TerminalInfo = () => {
  return (
    <Wrapper>
      <User>anonymous</User>@<WebsiteName>terminal.visper.dev</WebsiteName>:~$
    </Wrapper>
  );
};
