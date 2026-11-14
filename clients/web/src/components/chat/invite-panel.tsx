import { useState } from "react";
import styled from "styled-components";
import {
	formatInviteCode,
	formatInviteLink,
	type RoomKey,
} from "@/lib/room-crypto";

type Props = {
	joinCode: string;
	/** The room's join URL from the API (`qr_code_url`), without the key. */
	joinUrl: string;
	roomKey: RoomKey;
	onClose: () => void;
};

/**
 * What to share so others can join: the invite code (for a CLI or this
 * page's invite box) and the invite link. Both carry the room key, which the
 * server never sees.
 */
export const InvitePanel = ({ joinCode, joinUrl, roomKey, onClose }: Props) => {
	const code = formatInviteCode(joinCode, roomKey);
	const link = formatInviteLink(joinUrl, joinCode, roomKey);

	return (
		<Panel aria-label="Invite people">
			<Row>
				<Label>invite code</Label>
				<Value>{code}</Value>
				<CopyButton text={code} />
			</Row>
			{link && (
				<Row>
					<Label>invite link</Label>
					<Value>{link}</Value>
					<CopyButton text={link} />
				</Row>
			)}
			<Footer>
				<Note>
					Both carry the room's key: share them only with people you want in the
					room.
				</Note>
				<button type="button" onClick={onClose}>
					close
				</button>
			</Footer>
		</Panel>
	);
};

const CopyButton = ({ text }: { text: string }) => {
	const [state, setState] = useState<"idle" | "copied" | "failed">("idle");
	return (
		<Copy
			type="button"
			onClick={async () => {
				try {
					await navigator.clipboard.writeText(text);
					setState("copied");
				} catch {
					setState("failed");
				}
				window.setTimeout(() => setState("idle"), 1500);
			}}
		>
			{state === "copied" ? "copied ✓" : state === "failed" ? "✕" : "copy"}
		</Copy>
	);
};

const Panel = styled.section`
  display: flex;
  flex-direction: column;
  gap: 0.4rem;
  margin-top: 0.5rem;
  padding: 0.6rem 0.8rem;
  border: 1px solid ${({ theme }) => theme.colors.border};
  border-radius: 3px;
  background: ${({ theme }) => theme.colors.surfaceAlt};
`;

const Row = styled.div`
  display: grid;
  grid-template-columns: 7rem minmax(0, 1fr) auto;
  align-items: baseline;
  gap: 0.8rem;

  @media (max-width: 600px) {
    grid-template-columns: minmax(0, 1fr) auto;

    & > :first-child {
      grid-column: 1 / -1;
    }
  }
`;

const Label = styled.span`
  color: ${({ theme }) => theme.colors.text[300]};
`;

const Value = styled.code`
  overflow-wrap: anywhere;
  color: ${({ theme }) => theme.colors.primary};
  font-weight: 700;
`;

const Copy = styled.button`
  color: ${({ theme }) => theme.colors.text[200]};

  &:hover {
    color: ${({ theme }) => theme.colors.primary};
  }
`;

const Footer = styled.div`
  display: flex;
  flex-wrap: wrap;
  justify-content: space-between;
  gap: 0.5rem 1rem;

  button {
    color: ${({ theme }) => theme.colors.text[200]};
  }
`;

const Note = styled.span`
  color: ${({ theme }) => theme.colors.text[300]};
`;
