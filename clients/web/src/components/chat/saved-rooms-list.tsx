import { useEffect, useState } from "react";
import styled from "styled-components";
import { ChatApiError, chatApi } from "@/lib/chat-api";
import { RoomCrypto, RoomStateError } from "@/lib/room-keys";
import { forgetRoom, listSavedRooms, type SavedRoom } from "@/lib/saved-rooms";
import type { ChatSession } from "./join-form";

/** Statuses meaning we are no longer in the room. */
const GONE_STATUSES = [403, 404, 410];

/**
 * Reopens a remembered room: checks with the API that this browser is
 * still a member, as the same user, of a room with the owner pinned when it
 * was first entered, then restores its keys. Throws an Error to show;
 * forgets the room when it can never be reopened.
 */
async function reopen(saved: SavedRoom): Promise<ChatSession> {
	let crypto: RoomCrypto;
	try {
		crypto = RoomCrypto.restore(saved.crypto);
	} catch (e) {
		if (e instanceof RoomStateError) {
			await forgetRoom(saved.roomId);
			throw new Error("This room's saved keys are damaged: join it again.");
		}
		throw e;
	}

	let room: Awaited<ReturnType<typeof chatApi.get>>;
	try {
		room = await chatApi.get(saved.roomId);
	} catch (e) {
		if (e instanceof ChatApiError && GONE_STATUSES.includes(e.status)) {
			await forgetRoom(saved.roomId);
			throw new Error(
				"That room is gone, or you are no longer in it. Forgot it.",
			);
		}
		throw e;
	}

	if (room.current_user.id !== crypto.me.userId) {
		// The API's session cookie changed (expired or cleared): this browser
		// is someone else to the server now.
		await forgetRoom(saved.roomId);
		throw new Error(
			"This browser's session with the API changed, so it isn't in that room any more. Join with a new invite.",
		);
	}
	if (room.owner.id !== crypto.ownerId) {
		// Kept: it may be the server, not the room, that is wrong.
		throw new Error(
			"The server names a different owner than when you joined. Not reopening: ask the owner for a new invite.",
		);
	}
	return { room, key: crypto.currentKey(), crypto, remember: true };
}

/** The rooms this browser remembers, to reopen or forget. */
export const SavedRoomsList = ({
	onJoined,
}: {
	onJoined: (session: ChatSession) => void;
}) => {
	const [rooms, setRooms] = useState<SavedRoom[]>([]);
	const [busy, setBusy] = useState("");
	const [error, setError] = useState("");

	useEffect(() => {
		let stopped = false;
		listSavedRooms().then((r) => {
			if (!stopped) {
				setRooms(r);
			}
		});
		return () => {
			stopped = true;
		};
	}, []);

	if (rooms.length === 0 && !error) {
		return null;
	}

	const open = async (saved: SavedRoom) => {
		setBusy(saved.roomId);
		setError("");
		try {
			onJoined(await reopen(saved));
		} catch (e) {
			setError(e instanceof Error ? e.message : "Couldn't reopen the room.");
			setRooms(await listSavedRooms());
		} finally {
			setBusy("");
		}
	};

	const forget = async (saved: SavedRoom) => {
		if (
			!window.confirm(
				`Forget room #${saved.joinCode} on this browser? You'll need a new invite to read it again.`,
			)
		) {
			return;
		}
		await forgetRoom(saved.roomId);
		setRooms((rs) => rs.filter((r) => r.roomId !== saved.roomId));
	};

	return (
		<Section aria-labelledby="saved-rooms-title">
			<Label id="saved-rooms-title">rooms this browser remembers</Label>
			<List>
				{rooms.map((r) => (
					<li key={r.roomId}>
						<Code>#{r.joinCode}</Code>
						<Muted>
							{r.crypto.ownerId === r.userId
								? "yours"
								: `owner ${r.ownerName || "unknown"}`}
							{" · "}expires{" "}
							{new Date(r.expiresAt).toLocaleString([], {
								dateStyle: "short",
								timeStyle: "short",
							})}
						</Muted>
						<Buttons>
							<button
								type="button"
								className="primary"
								disabled={busy !== ""}
								onClick={() => open(r)}
							>
								{busy === r.roomId ? "opening…" : "open ↵"}
							</button>
							<button
								type="button"
								disabled={busy !== ""}
								onClick={() => forget(r)}
							>
								forget
							</button>
						</Buttons>
					</li>
				))}
			</List>
			{error && <ErrorText role="alert">✕ {error}</ErrorText>}
		</Section>
	);
};

const Section = styled.section`
  margin-bottom: 1.5rem;
`;

const Label = styled.div`
  margin-bottom: 0.25rem;
  color: ${({ theme }) => theme.colors.text[300]};
`;

const List = styled.ul`
  margin: 0;
  padding: 0;
  list-style: none;

  li {
    display: flex;
    flex-wrap: wrap;
    align-items: baseline;
    gap: 0.25rem 1rem;
  }
`;

const Code = styled.span`
  color: ${({ theme }) => theme.colors.primary};
  font-weight: 700;
`;

const Muted = styled.span`
  color: ${({ theme }) => theme.colors.text[300]};
`;

const Buttons = styled.span`
  display: flex;
  gap: 1rem;
  margin-left: auto;

  button {
    color: ${({ theme }) => theme.colors.text[200]};
  }

  button.primary {
    color: ${({ theme }) => theme.colors.primary};
  }

  button:disabled {
    opacity: 0.5;
  }
`;

const ErrorText = styled.p`
  margin: 0.25rem 0 0;
  color: ${({ theme }) => theme.colors.danger};
`;
