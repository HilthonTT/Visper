import type { ApiFileInfo } from "./chat-api";
import { type ChatState, memberName } from "./chat-state";
import { imageName, isEncryptedImageUrl } from "./room-crypto";

/** A row of the room's file list. */
export type RoomFile = {
	id: string;
	/** From the encrypted message pointing to the file; "" if none is loaded. */
	name: string;
	size: number;
	uploaderId: string;
	uploaderName: string;
	/** Milliseconds since the epoch (0 if unknown). */
	createdAt: number;
	/** The (loaded) message that shares the file, if any. */
	messageId?: string;
	/** Whether `me` may delete it: their own file, or any file if they own the room. */
	canDelete: boolean;
};

/**
 * Joins the server's file list with the loaded messages. The server only
 * knows each file as "file.enc"; its real name is in the encrypted message
 * that points to it (`<file url>#<name>`). Newest first.
 */
export function describeFiles(
	files: ApiFileInfo[],
	state: ChatState,
): RoomFile[] {
	const byUrl = new Map<string, { id: string; name: string }>();
	for (const m of state.messages) {
		if (m.failed || !isEncryptedImageUrl(m.content)) {
			continue;
		}
		const url = m.content.slice(0, m.content.indexOf("#"));
		byUrl.set(url, { id: m.id, name: imageName(m.content) });
	}

	return files
		.map((f) => {
			const shared = byUrl.get(f.url);
			return {
				id: f.id,
				name: shared?.name ?? "",
				size: f.size,
				uploaderId: f.uploader.id,
				uploaderName: f.uploader.username || memberName(state, f.uploader.id),
				createdAt: Date.parse(f.createdAt) || 0,
				messageId: shared?.id,
				canDelete: f.uploader.id === state.me || state.ownerId === state.me,
			};
		})
		.sort((a, b) => b.createdAt - a.createdAt);
}

/** A byte count for people: "512 B", "1.4 KB", "3.2 MB". */
export function formatSize(bytes: number): string {
	if (bytes < 1024) {
		return `${bytes} B`;
	}
	if (bytes < 1024 * 1024) {
		return `${(bytes / 1024).toFixed(1)} KB`;
	}
	return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}
