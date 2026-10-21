import { useEffect } from "react";
import { useTerminal } from "@/contexts/terminal-context";
import { useUser } from "@/contexts/user-context";
import { UsageDiv } from "../styles/output-styled";

export const SetUserId = () => {
	const { arg } = useTerminal();
	const { setUserId } = useUser();

	useEffect(() => {
		if (arg.length === 1) {
			setUserId(arg[0]);
		}
	}, [arg, setUserId]);

	if (arg.length !== 1) {
		return <UsageDiv>Usage: setuserid &lt;user_id&gt;</UsageDiv>;
	}

	return <div style={{ color: "#98c379" }}>User ID set to: {arg[0]}</div>;
};
