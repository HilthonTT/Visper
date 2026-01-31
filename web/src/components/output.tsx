import { OutputContainer, UsageDiv } from "./styles/output-styled";
import { About } from "./commands/about";
import { Clear } from "./commands/clear";
import { Echo } from "./commands/echo";
import { Help } from "./commands/help";
import { History } from "./commands/history";
import { Welcome } from "./commands/welcome";
import { GeneralOutput } from "./commands/general-output";
import { Themes } from "./commands/themes";
import { SetUserId } from "./commands/set-user-id";
import { SetSecureCode } from "./commands/set-secure-code";
import { SetJoinCode } from "./commands/set-join-code";
import { Env } from "./commands/env";
import { useTerminal } from "@/contexts/terminal-context";
import { JoinRoom } from "./commands/join-room";

const specialCmds: readonly string[] = [
  "themes",
  "echo",
  "setuserid",
  "setsecurecode",
  "setjoincode",
  "joinroom",
];

interface OutputProps {
  index: number;
  cmd: string;
}

export const Output = ({ index, cmd }: OutputProps) => {
  const { arg } = useTerminal();

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
          setuserid: <SetUserId />,
          setsecurecode: <SetSecureCode />,
          setjoincode: <SetJoinCode />,
          joinroom: <JoinRoom />,
          env: <Env />,
        }[cmd]
      }
    </OutputContainer>
  );
};
