import { createContext } from "react";

type Command = {
  cmd: string;
  desc: string;
  tab: number;
};

export const commands: Command[] = [
  { cmd: "about", desc: "about Sat Naing", tab: 8 },
  { cmd: "clear", desc: "clear the terminal", tab: 8 },
  { cmd: "echo", desc: "print out anything", tab: 9 },
  { cmd: "whoami", desc: "about current user", tab: 7 },
  { cmd: "help", desc: "check available commands", tab: 9 },
  { cmd: "history", desc: "view command history", tab: 6 },
  { cmd: "pwd", desc: "print current working directory", tab: 10 },
];

type Term = {
  arg: string[];
  history: string[];
  rerender: boolean;
  index: number;
  clearHistory?: () => void;
};

export const terminalContext = createContext<Term>({
  arg: [],
  history: [],
  rerender: false,
  index: 0,
});
