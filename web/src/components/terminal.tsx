import { useCallback, useEffect, useRef, useState } from "react";
import lodash from "lodash";
import { argTab } from "@/lib/funcs";
import {
  CmdNotFound,
  Empty,
  Form,
  Hints,
  Input,
  MobileBr,
  MobileSpan,
  Wrapper,
} from "./styles/terminal-styled";
import { TerminalInfo } from "./terminal-info";
import { Output } from "./output";
import type { RootSearchValues } from "@/schemas/root-schema";
import { useUser } from "@/contexts/user-context";
import { TerminalProvider } from "@/contexts/terminal-context";

type Command = {
  cmd: string;
  desc: string;
  tab: number;
};

export const commands: Command[] = [
  { cmd: "about", desc: "about Sat Naing", tab: 8 },
  { cmd: "clear", desc: "clear the terminal", tab: 8 },
  { cmd: "echo", desc: "print out anything", tab: 9 },
  { cmd: "env", desc: "print environment variables", tab: 10 },
  { cmd: "setuserid", desc: "sets the user id", tab: 4 },
  { cmd: "setjoincode", desc: "sets the room join code", tab: 2 },
  { cmd: "setsecurecode", desc: "sets the room secure code", tab: 0 },
  { cmd: "whoami", desc: "about current user", tab: 7 },
  { cmd: "help", desc: "check available commands", tab: 9 },
  { cmd: "history", desc: "view command history", tab: 6 },
  { cmd: "pwd", desc: "print current working directory", tab: 10 },
  { cmd: "welcome", desc: "display hero section", tab: 6 },
  { cmd: "themes", desc: "check available themes", tab: 7 },
];

interface TerminalProps {
  searchParams: RootSearchValues;
}

export const Terminal = ({ searchParams }: TerminalProps) => {
  const containerRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);

  const { setJoinCode, setSecureCode } = useUser();

  const [inputVal, setInputVal] = useState("");
  const [cmdHistory, setCmdHistory] = useState<string[]>(["welcome"]);
  const [rerender, setRerender] = useState(false);
  const [hints, setHints] = useState<string[]>([]);
  const [pointer, setPointer] = useState(-1);

  const handleChange = useCallback(
    (e: React.ChangeEvent<HTMLInputElement>) => {
      setRerender(false);
      setInputVal(e.target.value);
    },
    [inputVal],
  );

  const handleSubmit = (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    setCmdHistory([inputVal, ...cmdHistory]);
    setInputVal("");
    setRerender(true);
    setHints([]);
    setPointer(-1);
  };

  const clearHistory = () => {
    setCmdHistory([]);
    setHints([]);
  };

  const handleDivClick = () => {
    if (inputRef.current) {
      inputRef.current.focus();
    }
  };

  const handleKeyDown = (e: React.KeyboardEvent<HTMLInputElement>) => {
    setRerender(false);

    const ctrlI = e.ctrlKey && e.key.toLowerCase() === "i";
    const ctrlL = e.ctrlKey && e.key.toLowerCase() === "l";

    if (e.key === "Tab" || ctrlI) {
      e.preventDefault();
      if (!inputVal) {
        return;
      }

      let hintsCmds: string[] = [];

      commands.forEach(({ cmd }) => {
        if (lodash.startsWith(cmd, inputVal)) {
          hintsCmds = [...hintsCmds, cmd];
        }
      });

      const returnedHints = argTab(inputVal, setInputVal, setHints, hintsCmds);
      hintsCmds = returnedHints ? [...hintsCmds, ...returnedHints] : hintsCmds;

      if (hintsCmds.length > 1) {
        setHints(hintsCmds);
      } else if (hintsCmds.length === 1) {
        const currentCmd = lodash.split(inputVal, " ");
        setInputVal(
          currentCmd.length !== 1
            ? `${currentCmd[0]} ${currentCmd[1]} ${hintsCmds[0]}`
            : hintsCmds[0],
        );

        setHints([]);
      }
    }

    if (ctrlL) {
      clearHistory();
    }

    if (e.key === "ArrowUp") {
      if (pointer >= cmdHistory.length) {
        return;
      }

      if (pointer + 1 === cmdHistory.length) {
        return;
      }

      setInputVal(cmdHistory[pointer + 1]);
      setPointer((prevState) => prevState + 1);
      inputRef?.current?.blur();
    }

    if (e.key === "ArrowDown") {
      if (pointer < 0) {
        return;
      }

      if (pointer === 0) {
        setInputVal("");
        setPointer(-1);
        return;
      }

      setInputVal(cmdHistory[pointer - 1]);
      setPointer((prevState) => prevState - 1);
      inputRef?.current?.blur();
    }
  };

  useEffect(() => {
    document.addEventListener("click", handleDivClick);
    return () => {
      document.removeEventListener("click", handleDivClick);
    };
  }, [containerRef]);

  useEffect(() => {
    if (searchParams.joinCode) {
      setJoinCode(searchParams.joinCode);
    }
    if (searchParams.secureCode) {
      setSecureCode(searchParams.secureCode);
    }
  }, [
    searchParams.joinCode,
    searchParams.secureCode,
    setJoinCode,
    setSecureCode,
  ]);

  return (
    <Wrapper data-test-id="terminal-wrapper" ref={containerRef}>
      {hints.length > 1 && (
        <div>
          {hints.map((hCmd) => (
            <Hints key={hCmd}>{hCmd}</Hints>
          ))}
        </div>
      )}
      <Form onSubmit={handleSubmit}>
        <label htmlFor="terminal-input">
          <TerminalInfo /> <MobileBr />
          <MobileSpan>&#62;</MobileSpan>
        </label>
        <Input
          title="terminal-input"
          type="text"
          id="terminal-input"
          autoComplete="off"
          spellCheck="false"
          autoFocus
          autoCapitalize="off"
          ref={inputRef}
          value={inputVal}
          onKeyDown={handleKeyDown}
          onChange={handleChange}
        />
      </Form>

      {cmdHistory.map((cmdH, index) => {
        const commandArray = lodash.split(lodash.trim(cmdH), " ");
        const validCommand = lodash.find(commands, { cmd: commandArray[0] });
        const contextValue = {
          arg: lodash.drop(commandArray),
          history: cmdHistory,
          rerender,
          index,
          clearHistory,
        };
        return (
          <div key={lodash.uniqueId(`${cmdH}_`)}>
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
      })}
    </Wrapper>
  );
};
