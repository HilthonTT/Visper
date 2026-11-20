import { useCallback, useEffect, useState } from "react";
import styled from "styled-components";
import type { ApiFileInfo } from "@/lib/chat-api";
import type { ChatState } from "@/lib/chat-state";
import { describeFiles, formatSize } from "@/lib/room-files";

type Props = {
	state: ChatState;
	list: () => Promise<ApiFileInfo[]>;
	remove: (fileId: string) => Promise<unknown>;
	onJumpTo: (messageId: string) => void;
	onClose: () => void;
};

/**
 * The room's uploaded files. The server stores only ciphertext under a
 * meaningless name, so names come from the loaded messages that share each
 * file; older files show as "encrypted file".
 */
export const FilesPanel = ({
	state,
	list,
	remove,
	onJumpTo,
	onClose,
}: Props) => {
	const [files, setFiles] = useState<ApiFileInfo[] | null>(null);
	const [error, setError] = useState("");
	const [busy, setBusy] = useState("");

	const load = useCallback(async () => {
		setError("");
		try {
			setFiles(await list());
		} catch (e) {
			setError(e instanceof Error ? e.message : "Couldn't load the files.");
		}
	}, [list]);

	useEffect(() => {
		load();
	}, [load]);

	const rows = files ? describeFiles(files, state) : [];

	const onDelete = async (id: string, name: string) => {
		if (
			!window.confirm(
				`Delete ${name || "this file"} for everyone? Messages that show it will no longer load it.`,
			)
		) {
			return;
		}
		setBusy(id);
		setError("");
		try {
			await remove(id);
			setFiles((fs) => fs?.filter((f) => f.id !== id) ?? null);
		} catch (e) {
			setError(e instanceof Error ? e.message : "Couldn't delete the file.");
		} finally {
			setBusy("");
		}
	};

	return (
		<Panel aria-label="Room files">
			{files === null && !error && <Note>loading files…</Note>}
			{files !== null && rows.length === 0 && (
				<Note>No files shared in this room yet.</Note>
			)}
			{rows.length > 0 && (
				<List>
					{rows.map((f) => (
						<li key={f.id}>
							<Name title={f.name || "name not in the loaded messages"}>
								{f.messageId ? (
									<button
										type="button"
										onClick={() => f.messageId && onJumpTo(f.messageId)}
									>
										{f.name}
									</button>
								) : (
									<Muted>{f.name || "encrypted file"}</Muted>
								)}
							</Name>
							<Muted>{formatSize(f.size)}</Muted>
							<Muted>
								{f.uploaderId === state.me ? "you" : f.uploaderName}
							</Muted>
							<Muted>
								{f.createdAt
									? new Date(f.createdAt).toLocaleString([], {
											dateStyle: "short",
											timeStyle: "short",
										})
									: ""}
							</Muted>
							{f.canDelete ? (
								<Danger
									type="button"
									disabled={busy === f.id}
									onClick={() => onDelete(f.id, f.name)}
								>
									{busy === f.id ? "deleting…" : "delete"}
								</Danger>
							) : (
								<span />
							)}
						</li>
					))}
				</List>
			)}
			{error && <ErrorText role="alert">✕ {error}</ErrorText>}
			<Footer>
				<Note>Files are encrypted: the server can't see their names.</Note>
				<Buttons>
					<button type="button" onClick={load}>
						refresh
					</button>
					<button type="button" onClick={onClose}>
						close
					</button>
				</Buttons>
			</Footer>
		</Panel>
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

const List = styled.ul`
  display: flex;
  flex-direction: column;
  gap: 0.2rem;
  max-height: 12rem;
  margin: 0;
  padding: 0;
  overflow-y: auto;
  list-style: none;

  li {
    display: grid;
    grid-template-columns: minmax(0, 1fr) 5rem minmax(0, 8rem) 9rem 5rem;
    align-items: baseline;
    gap: 0.8rem;
    white-space: nowrap;

    @media (max-width: 600px) {
      grid-template-columns: minmax(0, 1fr) auto auto;

      & > :nth-child(3),
      & > :nth-child(4) {
        display: none;
      }
    }
  }
`;

const Name = styled.span`
  overflow: hidden;
  text-overflow: ellipsis;

  button {
    max-width: 100%;
    overflow: hidden;
    color: ${({ theme }) => theme.colors.primary};
    text-overflow: ellipsis;
  }
`;

const Muted = styled.span`
  overflow: hidden;
  color: ${({ theme }) => theme.colors.text[300]};
  text-overflow: ellipsis;
`;

const Danger = styled.button`
  justify-self: end;
  color: ${({ theme }) => theme.colors.danger};

  &:disabled {
    opacity: 0.6;
  }
`;

const ErrorText = styled.p`
  margin: 0;
  color: ${({ theme }) => theme.colors.danger};
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
  }

  button:hover {
    color: ${({ theme }) => theme.colors.primary};
  }
`;

const Note = styled.span`
  color: ${({ theme }) => theme.colors.text[300]};
`;
