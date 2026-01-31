import { generateTabs } from "@/lib/funcs";
import {
  Cmd,
  CmdDesc,
  CmdList,
  HelpWrapper,
  KeyContainer,
} from "../styles/help-styled";
import { commands } from "../terminal";
import { cn } from "@/lib/utils";

export const Help = () => {
  return (
    <HelpWrapper data-testid="help">
      {commands.map(({ cmd, desc, tab, special }) => (
        <CmdList key={cmd}>
          <Cmd className={cn(special && "text-amber-500!")}>{cmd}</Cmd>
          {generateTabs(tab)}
          <CmdDesc>- {desc}</CmdDesc>
        </CmdList>
      ))}
      <KeyContainer>
        <div>Tab or Ctrl + i&nbsp; =&gt; autocompletes the command</div>
        <div>Up Arrow {generateTabs(5)} =&gt; go back to previous command</div>
        <div>Ctrl + l {generateTabs(5)} =&gt; clear the terminal</div>
      </KeyContainer>
    </HelpWrapper>
  );
};
