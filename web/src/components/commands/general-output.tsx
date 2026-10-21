import type { ReactNode } from "react";
import { Wrapper } from "../styles/output-styled";

interface GeneralOutputProps {
	children: ReactNode;
}

export const GeneralOutput = ({ children }: GeneralOutputProps) => {
	return <Wrapper>{children}</Wrapper>;
};
