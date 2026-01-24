import { useContext } from "react";
import { terminalContext } from "./terminal";
import { OutputContainer, UsageDiv } from "./styles/output-styled";
import { About } from "./commands/about";
import { Clear } from "./commands/clear";
import { Echo } from "./commands/echo";
import { Help } from "./commands/help";
import { History } from "./commands/history";
import { Welcome } from "./commands/welcome";
import { GeneralOutput } from "./commands/general-output";
import { Themes } from "./commands/themes";

interface OutputProps {
  index: number;
  cmd: string;
}

export const Output = ({ index, cmd }: OutputProps) => {
  const { arg } = useContext(terminalContext);

  const specialCmds = ["themes", "echo"];

  if (!specialCmds.includes(cmd) && arg.length > 0) {
    return <UsageDiv data-testid="usage-output">Usage: {cmd}</UsageDiv>;
  }

  return (
    <OutputContainer data-testid={index === 0 ? "latest-output" : null}>
      {
        {
          about: <About />,
          clear: <Clear />,
          echo: <Echo />,
          help: <Help />,
          history: <History />,
          pwd: <GeneralOutput>/home/visper</GeneralOutput>,
          themes: <Themes />,
          welcome: <Welcome />,
          whoami: <GeneralOutput>anonymous</GeneralOutput>,
        }[cmd]
      }
    </OutputContainer>
  );
};
