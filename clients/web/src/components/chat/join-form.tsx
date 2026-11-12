import { useState } from "react";
import styled from "styled-components";
import { useUser } from "@/contexts/user-context";
import { type ApiRoom, chatApi } from "@/lib/chat-api";
import {
	InviteCodeError,
	parseInviteCode,
	type RoomKey,
	roomKeyFromUrl,
} from "@/lib/room-crypto";
import { RoomKeyError } from "@/lib/room-key";
import {
	Box,
	Button,
	Lead,
	PageContent,
	PageTitle,
} from "../pages/page-styles";

export type ChatSession = { room: ApiRoom; key: RoomKey };

type Status =
	| { kind: "idle" }
	| { kind: "joining" }
	| { kind: "error"; text: string };

/**
 * Joins a room in this browser, from an invite link (join code, secure code
 * and the key from the link's #fragment, already in the user context) or a
 * pasted invite code.
 */
export const JoinForm = ({
	onJoined,
}: {
	onJoined: (session: ChatSession) => void;
}) => {
	const user = useUser();
	const fromLink = Boolean(user.joinCode && user.secureCode && user.roomKey);
	const [invite, setInvite] = useState("");
	const [username, setUsername] = useState("");
	const [focused, setFocused] = useState<string | null>(null);
	const [status, setStatus] = useState<Status>({ kind: "idle" });

	const join = async (how: "link" | "code") => {
		setStatus({ kind: "joining" });
		const name = username.trim() || undefined;
		try {
			let room: ApiRoom;
			let key: RoomKey;
			if (how === "link") {
				key = roomKeyFromUrl(user.roomKey);
				room = await chatApi.joinWithToken(
					user.joinCode.trim(),
					user.secureCode.trim(),
					name,
				);
			} else {
				const parsed = parseInviteCode(invite);
				key = parsed.key;
				room = await chatApi.joinByCode(parsed.joinCode, name);
			}
			onJoined({ room, key });
		} catch (e) {
			const text =
				e instanceof InviteCodeError || e instanceof RoomKeyError
					? e.message
					: e instanceof Error
						? e.message
						: "Couldn't join the room.";
			setStatus({ kind: "error", text });
		}
	};

	const joining = status.kind === "joining";

	return (
		<PageContent>
			<PageTitle>Chat in this browser</PageTitle>
			<Lead>
				Messages are encrypted here with the room's key before they leave the
				page; the server only stores ciphertext.
			</Lead>

			{fromLink && (
				<Section>
					<Label>invite link</Label>
					<p>
						Room <strong>{user.joinCode}</strong>, key from the link.
					</p>
					<Button type="button" disabled={joining} onClick={() => join("link")}>
						Join here ↵
					</Button>
				</Section>
			)}

			<form
				onSubmit={(e) => {
					e.preventDefault();
					join("code");
				}}
			>
				<Section>
					<label htmlFor="invite-code">
						<Label>
							{fromLink ? "or paste an invite code" : "invite code"}
						</Label>
						<Box
							$active={focused === "invite"}
							$tone={status.kind === "error" ? "danger" : undefined}
						>
							<Prompt>›</Prompt>
							<Input
								id="invite-code"
								autoComplete="off"
								spellCheck={false}
								placeholder="ABC123.… (ctrl+p in a CLI room)"
								value={invite}
								onFocus={() => setFocused("invite")}
								onBlur={() => setFocused(null)}
								onChange={(e) => {
									setInvite(e.target.value);
									if (status.kind === "error") {
										setStatus({ kind: "idle" });
									}
								}}
							/>
						</Box>
					</label>
				</Section>
				<Section>
					<label htmlFor="chat-username">
						<Label>name (optional)</Label>
						<Box $active={focused === "name"}>
							<Prompt>›</Prompt>
							<Input
								id="chat-username"
								autoComplete="off"
								maxLength={50}
								placeholder="a random name if empty"
								value={username}
								onFocus={() => setFocused("name")}
								onBlur={() => setFocused(null)}
								onChange={(e) => setUsername(e.target.value)}
							/>
						</Box>
					</label>
				</Section>
				<Actions>
					<Button type="submit" disabled={joining || !invite.trim()}>
						Join ↵
					</Button>
					<StatusText $error={status.kind === "error"} role="status">
						{status.kind === "joining" && "Joining…"}
						{status.kind === "error" && `✕ ${status.text}`}
					</StatusText>
				</Actions>
			</form>
			<Aside>
				Rather chat in your terminal? The <strong>Join room</strong> page (j)
				sends this invite to your CLI.
			</Aside>
		</PageContent>
	);
};

const Section = styled.div`
  margin-bottom: 1rem;

  p {
    margin-bottom: 0.5rem;
    color: ${({ theme }) => theme.colors.text[200]};
  }

  strong {
    color: ${({ theme }) => theme.colors.primary};
  }
`;

const Aside = styled.p`
  margin-top: 2rem;
  color: ${({ theme }) => theme.colors.text[300]};

  strong {
    color: ${({ theme }) => theme.colors.text[200]};
  }
`;

const Label = styled.div`
  margin-bottom: 0.25rem;
  color: ${({ theme }) => theme.colors.text[300]};
`;

const Prompt = styled.span`
  margin-right: 0.6rem;
  color: ${({ theme }) => theme.colors.primary};
  font-weight: 700;
`;

const Input = styled.input`
  width: calc(100% - 1.5rem);
  padding: 0;
  border: none;
  font-weight: 700;
`;

const Actions = styled.div`
  display: flex;
  flex-wrap: wrap;
  align-items: baseline;
  gap: 0.5rem 1rem;
`;

const StatusText = styled.span<{ $error: boolean }>`
  color: ${({ theme, $error }) =>
		$error ? theme.colors.danger : theme.colors.primary};
`;
