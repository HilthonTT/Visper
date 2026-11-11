import { useEffect } from "react";
import { useTerminal } from "@/contexts/terminal-context";
import { useUser } from "@/contexts/user-context";
import { UsageDiv } from "../styles/output-styled";

export const SetSecureCode = () => {
	const { arg } = useTerminal();
	const { setSecureCode } = useUser();

	useEffect(() => {
		if (arg.length === 1) {
			setSecureCode(arg[0]);
		}
	}, [arg, setSecureCode]);

	if (arg.length !== 1) {
		return <UsageDiv>Usage: setsecurecode &lt;secure_code&gt;</UsageDiv>;
	}

	return <div style={{ color: "#98c379" }}>Secure code set successfully</div>;
};
