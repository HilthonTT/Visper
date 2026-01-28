import { useEffect } from "react";

import { UsageDiv } from "../styles/output-styled";
import { useUser } from "@/contexts/user-context";
import { useTerminal } from "@/contexts/terminal-context";

export const SetJoinCode = () => {
  const { arg } = useTerminal();
  const { setJoinCode } = useUser();

  useEffect(() => {
    if (arg.length === 1) {
      setJoinCode(arg[0]);
    }
  }, [arg, setJoinCode]);

  if (arg.length !== 1) {
    return <UsageDiv>Usage: setjoincode &lt;join_code&gt;</UsageDiv>;
  }

  return <div style={{ color: "#98c379" }}>Join code set to: {arg[0]}</div>;
};
