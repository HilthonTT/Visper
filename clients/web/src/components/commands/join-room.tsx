import { useEffect, useRef, useState } from "react";
import { useUser } from "@/contexts/user-context";
import { notifySelfRoomInvite } from "@/lib/api";
import { type Status, StatusLine } from "../styles/output-styled";

export const JoinRoom = () => {
	const user = useUser();
	// Snapshot the credentials at the time the command was run, so later
	// `set*` commands don't retroactively change this entry.
	const credentials = useRef(user).current;

	const [status, setStatus] = useState<Status>("loading");
	const [message, setMessage] = useState("");

	useEffect(() => {
		const { deviceCode, joinCode, secureCode, roomKey } = credentials;

		const missing = !deviceCode
			? "Device code not set. Use 'setdevicecode <code>' first (from your CLI's settings page)."
			: !joinCode
				? "Join code not set. Use 'setjoincode <code>' first."
				: !secureCode
					? "Secure code not set. Use 'setsecurecode <code>' first."
					: !roomKey
						? "Room key not set. Open the invite link, or use 'setroomkey <key>'."
						: null;

		if (missing) {
			setStatus("error");
			setMessage(`Error: ${missing}`);
			return;
		}

		const controller = new AbortController();
		setStatus("loading");
		setMessage("Sending notification to your devices...");

		notifySelfRoomInvite(
			{ deviceCode, joinCode, secureCode, roomKey },
			controller.signal,
		)
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
		<StatusLine $status={status}>
			<div>
				{message.split("\n").map((line, i) => (
					// biome-ignore lint/suspicious/noArrayIndexKey: static lines
					<div key={i}>{line || " "}</div>
				))}
			</div>
		</StatusLine>
	);
};
