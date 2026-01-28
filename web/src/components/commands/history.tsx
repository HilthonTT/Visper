import lodash from "lodash";
import { Wrapper } from "../styles/output-styled";
import { useTerminal } from "@/contexts/terminal-context";

export const History = () => {
  const { history, index } = useTerminal();
  const currentHistory = lodash.reverse(lodash.slice(history, index));

  return (
    <Wrapper data-test-id="history">
      {currentHistory.map((cmd) => (
        <div key={lodash.uniqueId(`${cmd}_`)}>{cmd}</div>
      ))}
    </Wrapper>
  );
};
