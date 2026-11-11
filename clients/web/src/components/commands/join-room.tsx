import { useEffect, useRef, useState } from "react";
import { useUser } from "@/contexts/user-context";
import { notifySelfRoomInvite } from "@/lib/api";
import { Wrapper } from "../styles/output-styled";

type Status = "loading" | "success" | "error";

const statusColors: Record<Status, string> = {
	loading: "#FFD700",
	error: "#FF6B6B",
	success: "#51CF66",
};

export const JoinRoom = () => {
	const user = useUser();
	// Snapshot the credentials at the time the command was run, so later
	// `set*` commands don't retroactively change this entry.
	const credentials = useRef(user).current;

	const [status, setStatus] = useState<Status>("loading");
	const [message, setMessage] = useState("");

	useEffect(() => {
		const { userId, joinCode, secureCode } = credentials;

		const missing = !userId
			? "User ID not set. Use 'setuserid <your-id>' first."
			: !joinCode
				? "Join code not set. Use 'setjoincode <code>' first."
				: !secureCode
					? "Secure code not set. Use 'setsecurecode <code>' first."
					: null;

		if (missing) {
			setStatus("error");
			setMessage(`Error: ${missing}`);
			return;
		}

		const controller = new AbortController();
		setStatus("loading");
		setMessage("Sending notification to your devices...");

		notifySelfRoomInvite({ userId, joinCode, secureCode }, controller.signal)
			.then((data) => {
				setStatus("success");
				setMessage(
					data.already_joined
						? `You are already a member of room ${data.room_id}`
						: `Notification sent successfully!\nRoom: ${data.room_id}\nUser: ${data.username}\n\nCheck your CLI for the room invite notification.`,
				);
			})
			.catch((error: unknown) => {
				if (controller.signal.aborted) {
					return;
				}
				setStatus("error");
				setMessage(
					`Error: ${error instanceof Error ? error.message : "Unknown error"}`,
				);
			});

		// Abort on unmount (e.g. `clear`) so we never set state afterwards.
		return () => controller.abort();
	}, [credentials]);

	return (
		<Wrapper style={{ color: statusColors[status] }}>
			{message.split("\n").map((line, i) => (
				// biome-ignore lint/suspicious/noArrayIndexKey: static lines
				<div key={i}>{line || " "}</div>
			))}
		</Wrapper>
	);
};
