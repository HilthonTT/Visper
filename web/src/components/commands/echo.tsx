import lodash from "lodash";
import { Wrapper } from "@/components/styles/output-styled";
import { useEnvVariables } from "@/hooks/use-env-variables";
import { useTerminal } from "@/contexts/terminal-context";

export const Echo = () => {
  const { arg } = useTerminal();
  const variables = useEnvVariables();

  let outputStr = lodash.join(arg, " ");
  outputStr = lodash.trim(outputStr, "'"); // remove trailing single quotes ''
  outputStr = lodash.trim(outputStr, '"'); // remove trailing double quotes ""
  outputStr = lodash.trim(outputStr, "`"); // remove trailing backtick ``

  // Replace environment variable references in the format $VAR or ${VAR}
  // Pattern breakdown:
  // \$       - Match literal dollar sign
  // \{?      - Optionally match opening brace (for ${VAR} syntax)
  // (\w+)    - Capture one or more word characters (variable name)
  // \}?      - Optionally match closing brace
  // g        - Global flag: replace all occurrences
  outputStr = outputStr.replace(/\$\{?(\w+)\}?/g, (match, varName) => {
    return variables[varName as keyof typeof variables] !== undefined
      ? variables[varName as keyof typeof variables]
      : match;
  });

  return <Wrapper>{outputStr}</Wrapper>;
};
