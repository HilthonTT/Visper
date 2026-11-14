import { memo, useState } from "react";
import styled from "styled-components";
import { type ChatMessage, groupReactions, myReaction } from "@/lib/chat-state";
import {
	isEncryptedImageUrl,
	isPlainImageUrl,
	type RoomKey,
} from "@/lib/room-crypto";
import { ImageMessage } from "./image-message";

/** The reactions offered in the picker. */
export const REACTIONS = ["👍", "❤️", "😂", "😮", "😢", "🎉"] as const;

type Props = {
	message: ChatMessage;
	me: string;
	roomKey: RoomKey;
	/** The message this one replies to; null if it is gone. */
	replyTo: ChatMessage | null | undefined;
	seenBy: string[] | undefined;
	onReply: (m: ChatMessage) => void;
	onEdit: (m: ChatMessage) => void;
	onDelete: (m: ChatMessage) => void;
	onReact: (m: ChatMessage, emoji: string) => void;
	onJumpTo: (id: string) => void;
};

const time = (ms: number) =>
	new Date(ms).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });

export const MessageItem = memo(function MessageItem({
	message,
	me,
	roomKey,
	replyTo,
	seenBy,
	onReply,
	onEdit,
	onDelete,
	onReact,
	onJumpTo,
}: Props) {
	const [picking, setPicking] = useState(false);
	const own = message.userId === me;
	const mine = myReaction(message.reactions, me);
	const image =
		!message.failed &&
		(isEncryptedImageUrl(message.content) || isPlainImageUrl(message.content));

	return (
		<Item id={`msg-${message.id}`} $own={own}>
			<Meta>
				<Author $own={own}>{own ? "you" : message.username}</Author>
				<Time>{time(message.createdAt)}</Time>
				{message.edited && <Time>(edited)</Time>}
			</Meta>

			{message.replyToId !== undefined && (
				<Quote
					type="button"
					disabled={!replyTo}
					onClick={() => replyTo && onJumpTo(replyTo.id)}
				>
					↪{" "}
					{replyTo ? (
						<>
							<strong>
								{replyTo.userId === me ? "you" : replyTo.username}
							</strong>{" "}
							{snippet(replyTo)}
						</>
					) : (
						<em>original message deleted</em>
					)}
				</Quote>
			)}

			{image ? (
				<ImageMessage
					// Remount when an edit changes the image, so no stale state is kept.
					key={message.content}
					content={message.content}
					roomKey={roomKey}
				/>
			) : (
				<Content $failed={message.failed}>{message.content}</Content>
			)}

			{(message.reactions.length > 0 || picking) && (
				<Reactions>
					{groupReactions(message.reactions, me).map((g) => (
						<ReactionChip
							key={g.emoji}
							type="button"
							$mine={g.mine}
							title={g.mine ? "remove your reaction" : "react with this"}
							onClick={() => onReact(message, g.emoji)}
						>
							{g.emoji} {g.count}
						</ReactionChip>
					))}
					{picking &&
						REACTIONS.filter(
							(e) => !message.reactions.some((r) => r.emoji === e),
						).map((emoji) => (
							<ReactionChip
								key={emoji}
								type="button"
								$mine={false}
								aria-label={`react with ${emoji}`}
								onClick={() => {
									setPicking(false);
									onReact(message, emoji);
								}}
							>
								{emoji}
							</ReactionChip>
						))}
				</Reactions>
			)}

			<Actions className="actions">
				<Action type="button" onClick={() => onReply(message)}>
					reply
				</Action>
				<Action
					type="button"
					aria-expanded={picking}
					onClick={() => setPicking((p) => !p)}
				>
					{picking ? "close" : mine ? `react (${mine})` : "react"}
				</Action>
				{own && !image && (
					<Action type="button" onClick={() => onEdit(message)}>
						edit
					</Action>
				)}
				{own && (
					<Action type="button" $danger onClick={() => onDelete(message)}>
						delete
					</Action>
				)}
			</Actions>

			{seenBy && seenBy.length > 0 && <Seen>seen by {seenBy.join(", ")}</Seen>}
		</Item>
	);
});

function snippet(m: ChatMessage): string {
	if (isEncryptedImageUrl(m.content) || isPlainImageUrl(m.content)) {
		return "[image]";
	}
	return m.content.length > 80 ? `${m.content.slice(0, 80)}…` : m.content;
}

const Item = styled.article<{ $own: boolean }>`
  position: relative;
  padding: 0.35rem 0.6rem;
  border-left: 2px solid
    ${({ theme, $own }) => ($own ? theme.colors.primary : "transparent")};
  border-radius: 3px;

  &:hover,
  &:focus-within {
    background: ${({ theme }) => theme.colors.surfaceAlt};
  }

  &.highlight {
    background: color-mix(in srgb, ${({ theme }) => theme.colors.primary} 15%, transparent);
  }

  .actions {
    visibility: hidden;
  }

  &:hover .actions,
  &:focus-within .actions {
    visibility: visible;
  }

  /* No hover on touch screens: always show the actions. */
  @media (hover: none) {
    .actions {
      visibility: visible;
    }
  }
`;

const Meta = styled.div`
  display: flex;
  flex-wrap: wrap;
  gap: 0.5rem;
  align-items: baseline;
`;

const Author = styled.span<{ $own: boolean }>`
  color: ${({ theme, $own }) =>
		$own ? theme.colors.primary : theme.colors.secondary};
  font-weight: 700;
`;

const Time = styled.span`
  color: ${({ theme }) => theme.colors.text[300]};
  font-size: 0.85em;
`;

const Quote = styled.button`
  display: block;
  max-width: 100%;
  margin: 0.15rem 0;
  padding-left: 0.5rem;
  border-left: 2px solid ${({ theme }) => theme.colors.border};
  color: ${({ theme }) => theme.colors.text[200]};
  font-size: 0.9em;
  text-align: left;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;

  strong {
    color: ${({ theme }) => theme.colors.text[100]};
  }

  &:disabled {
    cursor: default;
  }
`;

const Content = styled.div<{ $failed: boolean }>`
  color: ${({ theme, $failed }) =>
		$failed ? theme.colors.danger : theme.colors.text[100]};
  white-space: pre-wrap;
  overflow-wrap: anywhere;
`;

const Reactions = styled.div`
  display: flex;
  flex-wrap: wrap;
  gap: 0.3rem;
  margin-top: 0.3rem;
`;

const ReactionChip = styled.button<{ $mine: boolean }>`
  padding: 0 0.45rem;
  border: 1px solid
    ${({ theme, $mine }) => ($mine ? theme.colors.primary : theme.colors.border)};
  border-radius: 999px;
  background: ${({ theme }) => theme.colors.surfaceAlt};
  color: ${({ theme }) => theme.colors.text[100]};
  line-height: 1.6;
`;

const Actions = styled.div`
  display: flex;
  gap: 0.75rem;
  margin-top: 0.15rem;
  font-size: 0.85em;
`;

const Action = styled.button<{ $danger?: boolean }>`
  color: ${({ theme, $danger }) =>
		$danger ? theme.colors.danger : theme.colors.text[300]};

  &:hover,
  &:focus-visible {
    color: ${({ theme, $danger }) =>
			$danger ? theme.colors.danger : theme.colors.primary};
  }
`;

const Seen = styled.div`
  margin-top: 0.1rem;
  color: ${({ theme }) => theme.colors.text[300]};
  font-size: 0.8em;
  text-align: right;
`;
