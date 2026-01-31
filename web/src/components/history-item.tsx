import { memo, useMemo } from "react";
import lodash from "lodash";
import { TerminalInfo } from "./terminal-info";
import {
  CmdNotFound,
  Empty,
  MobileBr,
  MobileSpan,
} from "./styles/terminal-styled";
import { TerminalProvider } from "@/contexts/terminal-context";
import { Output } from "./output";

interface HistoryItemProps {
  cmdH: string;
  index: number;
  validCommand: any;
  commandArray: string[];
  cmdHistory: string[];
  rerender: boolean;
  clearHistory: () => void;
}

export const HistoryItem = memo(
  ({
    cmdH,
    index,
    validCommand,
    commandArray,
    cmdHistory,
    rerender,
    clearHistory,
  }: HistoryItemProps) => {
    const contextValue = useMemo(
      () => ({
        arg: lodash.drop(commandArray),
        history: cmdHistory,
        rerender,
        index,
        clearHistory,
      }),
      [commandArray, cmdHistory, rerender, index, clearHistory],
    );

    return (
      <div>
        <div>
          <TerminalInfo />
          <MobileBr />
          <MobileSpan>&#62;</MobileSpan>
          <span data-testid="input-command">{cmdH}</span>
        </div>
        {validCommand ? (
          <TerminalProvider value={contextValue}>
            <Output index={index} cmd={commandArray[0]} />
          </TerminalProvider>
        ) : cmdH === "" ? (
          <Empty />
        ) : (
          <CmdNotFound data-testid={`not-found-${index}`}>
            command not found: {cmdH}
          </CmdNotFound>
        )}
      </div>
    );
  },
);

HistoryItem.displayName = "HistoryItem";
