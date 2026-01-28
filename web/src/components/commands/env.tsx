import { Wrapper } from "@/components/styles/output-styled";
import { useEnvVariables } from "@/hooks/use-env-variables";

export const Env = () => {
  const variables = useEnvVariables();

  return (
    <Wrapper>
      {Object.entries(variables).map(([name, value]) => (
        <div key={name}>
          {name}={value || "(not set)"}
        </div>
      ))}
    </Wrapper>
  );
};
