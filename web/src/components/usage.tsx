import { UsageDiv } from "./styles/output-styled";

interface UsageProps {
  cmd: "themes";
  marginY?: boolean;
}

const arg = {
  themes: { placeholder: "theme-name", example: "ubuntu" },
};

export const Usage = ({ cmd, marginY }: UsageProps) => {
  const action = cmd === "themes" ? "set" : "go";

  return (
    <UsageDiv data-testid={`${cmd}-invalid-arg`} marginY={marginY}>
      Usage: {cmd} {action} &#60;{arg[cmd].placeholder}&#62; <br />
      eg: {cmd} {action} {arg[cmd].example}
    </UsageDiv>
  );
};
