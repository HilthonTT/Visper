import styled from "styled-components";
import type { KeyStatus } from "@/lib/room-keys";

type Props = {
	/** The member whose key is being checked; null to show only ours. */
	member: { id: string; username: string } | null;
	myFingerprint: string;
	/** The member's fingerprint ("" if no key card is known for them). */
	fingerprint: string;
	status: KeyStatus;
	/**
	 * Pins the member's key. Gets the fingerprint shown, so the key pinned is
	 * the one the user compared, not a card that arrived since.
	 */
	onVerify: (fingerprint: string) => void;
	onUnverify: () => void;
	onClose: () => void;
};

/**
 * Compares a member's key fingerprint with what they see for themselves.
 * The server relays key cards, so it could swap one in someone's name;
 * reading the digits to each other over a call or in person rules that out.
 * Verified members are pinned: a different key for them is refused.
 */
export const KeysPanel = ({
	member,
	myFingerprint,
	fingerprint,
	status,
	onVerify,
	onUnverify,
	onClose,
}: Props) => (
	<Panel aria-label="Verify keys">
		<Row>
			<Label>your key</Label>
			<Value>{myFingerprint}</Value>
		</Row>
		{member && (
			<Row>
				<Label>{member.username}</Label>
				{fingerprint ? (
					<Value $tone={status === "changed" ? "warning" : undefined}>
						{fingerprint}
					</Value>
				) : (
					<Note>no key yet: they haven't published one</Note>
				)}
				<Status $status={status}>
					{status === "verified"
						? "✓ verified"
						: status === "changed"
							? "! key changed"
							: "unverified"}
				</Status>
			</Row>
		)}
		{member && status === "changed" && (
			<Warning role="alert">
				{member.username}'s key changed since you verified it. Until you check
				the new one, only their old key is trusted: messages signed with the new
				key show as unverified, and new room keys go to the old one. Their
				client may have restarted, or someone may be posing as them.
			</Warning>
		)}
		<Footer>
			<Note>
				{member
					? `Ask ${member.username} to read out "your key" from their screen, over a call or in person. Mark them verified only if it matches the digits above.`
					: "Pick a member to compare keys with them. Your fingerprint changes when you reload this page, unless this browser remembers the room."}
			</Note>
			<Buttons>
				{member && fingerprint && status !== "verified" && (
					<button
						type="button"
						className="primary"
						onClick={() => onVerify(fingerprint)}
					>
						{status === "changed" ? "verify new key" : "mark verified"}
					</button>
				)}
				{member && status !== "unverified" && (
					<button type="button" onClick={onUnverify}>
						unverify
					</button>
				)}
				<button type="button" onClick={onClose}>
					close
				</button>
			</Buttons>
		</Footer>
	</Panel>
);

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
  overflow: hidden;
  color: ${({ theme }) => theme.colors.text[300]};
  text-overflow: ellipsis;
  white-space: nowrap;
`;

const Value = styled.code<{ $tone?: "warning" }>`
  overflow-wrap: anywhere;
  color: ${({ theme, $tone }) =>
		$tone === "warning" ? theme.colors.warning : theme.colors.primary};
  font-weight: 700;
  letter-spacing: 0.04em;
`;

const Status = styled.span<{ $status: KeyStatus }>`
  color: ${({ theme, $status }) =>
		$status === "verified"
			? theme.colors.success
			: $status === "changed"
				? theme.colors.warning
				: theme.colors.text[300]};
  white-space: nowrap;
`;

const Warning = styled.p`
  margin: 0;
  color: ${({ theme }) => theme.colors.warning};
`;

const Footer = styled.div`
  display: flex;
  flex-wrap: wrap;
  justify-content: space-between;
  gap: 0.5rem 1rem;
`;

const Buttons = styled.div`
  display: flex;
  gap: 1rem;
  margin-left: auto;

  button {
    color: ${({ theme }) => theme.colors.text[200]};
    white-space: nowrap;
  }

  button.primary {
    color: ${({ theme }) => theme.colors.success};
  }

  button:hover {
    color: ${({ theme }) => theme.colors.primary};
  }
`;

const Note = styled.span`
  color: ${({ theme }) => theme.colors.text[300]};
`;
