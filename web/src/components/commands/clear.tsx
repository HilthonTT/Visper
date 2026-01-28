import { useEffect } from "react";
import { UsageDiv } from "../styles/output-styled";
import { useTerminal } from "@/contexts/terminal-context";

export const Clear = () => {
  const { arg, clearHistory } = useTerminal();

  useEffect(() => {
    if (arg.length < 1) {
      clearHistory?.();
    }
  }, [arg, clearHistory]);

  if (arg.length > 0) {
    return <UsageDiv>Usage: clear</UsageDiv>;
  }

  return <></>;
};
