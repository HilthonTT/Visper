import { Wrapper } from "../styles/output-styled";

interface GeneralOutputProps {
  children: React.ReactNode;
}

export const GeneralOutput = ({ children }: GeneralOutputProps) => {
  return <Wrapper>{children}</Wrapper>;
};
