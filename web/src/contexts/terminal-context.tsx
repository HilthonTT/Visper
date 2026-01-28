import { createContext, useContext } from "react";

type TerminalContextType = {
  arg: string[];
  history: string[];
  rerender: boolean;
  index: number;
  clearHistory?: () => void;
};

const TerminalContext = createContext<TerminalContextType>({
  arg: [],
  history: [],
  rerender: false,
  index: 0,
});

interface TerminalProviderProps {
  value: TerminalContextType;
  children: React.ReactNode;
}

export const TerminalProvider = ({
  value,
  children,
}: TerminalProviderProps) => {
  return (
    <TerminalContext.Provider value={value}>
      {children}
    </TerminalContext.Provider>
  );
};

export const useTerminal = () => {
  const context = useContext(TerminalContext);
  if (!context) {
    throw new Error("useTerminal must be used within TerminalProvider");
  }
  return context;
};
