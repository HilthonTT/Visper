import { useContext } from "react";
import { terminalContext } from "../terminal";
import lodash from "lodash";
import { Wrapper } from "../styles/output-styled";

export const History = () => {
  const { history, index } = useContext(terminalContext);
  const currentHistory = lodash.reverse(lodash.slice(history, index));

  return (
    <Wrapper data-test-id="history">
      {currentHistory.map((cmd) => (
        <div key={lodash.uniqueId(`${cmd}_`)}>{cmd}</div>
      ))}
    </Wrapper>
  );
};
