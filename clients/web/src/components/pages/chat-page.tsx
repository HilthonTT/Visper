import { useState } from "react";
import { ChatView } from "../chat/chat-view";
import { type ChatSession, JoinForm } from "../chat/join-form";

/**
 * Chat in the browser: join a room (invite link or invite code), then the
 * room itself. Stays mounted while other pages are shown so the connection
 * survives tab switches. The room key lives in memory only, so a reload
 * means joining again.
 */
export const ChatPage = ({ active }: { active: boolean }) => {
	const [session, setSession] = useState<ChatSession | null>(null);

	if (!session) {
		return <JoinForm onJoined={setSession} />;
	}
	return (
		<ChatView
			key={session.room.id}
			session={session}
			active={active}
			onLeft={() => setSession(null)}
		/>
	);
};
