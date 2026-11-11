import { type ReactNode, useEffect } from "react";
import { useTerminal } from "@/contexts/terminal-context";
import { useUser } from "@/contexts/user-context";
import { StatusLine, UsageDiv } from "../styles/output-styled";

interface SetCredentialProps {
	cmd: string;
	placeholder: string;
	/** Must be stable (state setters are), or the effect re-runs every render. */
	setValue: (value: string) => void;
	success: (value: string) => ReactNode;
}

const SetCredential = ({
	cmd,
	placeholder,
	setValue,
	success,
}: SetCredentialProps) => {
	const { arg } = useTerminal();

	useEffect(() => {
		if (arg.length === 1) {
			setValue(arg[0]);
		}
	}, [arg, setValue]);

	if (arg.length !== 1) {
		return (
			<UsageDiv>
				Usage: {cmd} &lt;{placeholder}&gt;
			</UsageDiv>
		);
	}

	return <StatusLine $status="success">{success(arg[0])}</StatusLine>;
};

export const SetUserId = () => (
	<SetCredential
		cmd="setuserid"
		placeholder="user_id"
		setValue={useUser().setUserId}
		success={(id) => `User ID set to: ${id}`}
	/>
);

export const SetJoinCode = () => (
	<SetCredential
		cmd="setjoincode"
		placeholder="join_code"
		setValue={useUser().setJoinCode}
		success={(code) => `Join code set to: ${code}`}
	/>
);

export const SetSecureCode = () => (
	<SetCredential
		cmd="setsecurecode"
		placeholder="secure_code"
		setValue={useUser().setSecureCode}
		success={() => "Secure code set successfully"}
	/>
);
