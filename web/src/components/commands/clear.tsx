import { useContext, useEffect } from "react";
import { terminalContext } from "../terminal";
import { UsageDiv } from "../styles/output-styled";

export const Clear = () => {
  const { arg, clearHistory } = useContext(terminalContext);

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
