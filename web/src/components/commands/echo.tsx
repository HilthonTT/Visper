import lodash from "lodash";
import { useContext } from "react";
import { Wrapper } from "@/components/styles/output-styled";
import { terminalContext } from "../terminal";

export const Echo = () => {
  const { arg } = useContext(terminalContext);

  let outputStr = lodash.join(arg, " ");
  outputStr = lodash.trim(outputStr, "'"); // remove trailing single quotes ''
  outputStr = lodash.trim(outputStr, '"'); // remove trailing double quotes ""
  outputStr = lodash.trim(outputStr, "`"); // remove trailing backtick ``

  return <Wrapper>{outputStr}</Wrapper>;
};
