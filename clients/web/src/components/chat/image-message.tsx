import { useEffect, useState } from "react";
import styled from "styled-components";
import {
	imageMimeType,
	imageName,
	isEncryptedImageUrl,
	MAX_UPLOAD_SIZE,
} from "@/lib/room-crypto";
import { FileMismatchError, type RoomCrypto } from "@/lib/room-keys";

/** The API stores nothing larger, so a bigger download isn't one of ours. */
const MAX_IMAGE_BYTES = MAX_UPLOAD_SIZE;

/** Reads a response body, failing as soon as it passes `max` bytes. */
async function readCapped(res: Response, max: number): Promise<Uint8Array> {
	const declared = Number(res.headers.get("Content-Length"));
	if (declared > max) {
		throw new Error("image too large");
	}
	if (!res.body) {
		throw new Error("empty response");
	}
	const reader = res.body.getReader();
	const chunks: Uint8Array[] = [];
	let total = 0;
	for (;;) {
		const { done, value } = await reader.read();
		if (done) {
			break;
		}
		total += value.length;
		if (total > max) {
			await reader.cancel();
			throw new Error("image too large");
		}
		chunks.push(value);
	}
	const data = new Uint8Array(total);
	let offset = 0;
	for (const chunk of chunks) {
		data.set(chunk, offset);
		offset += chunk.length;
	}
	return data;
}

type Status =
	| { kind: "loading" }
	| { kind: "ready"; src: string }
	| { kind: "error" }
	| { kind: "mismatch" };

/**
 * An image message. Encrypted images (".enc" URL with the name in the
 * fragment) are downloaded, checked against the hash their message carries,
 * decrypted with the room's keys and shown from a blob URL; plain image
 * links are shown as they are.
 */
export const ImageMessage = ({
	content,
	fileHash,
	crypto,
}: {
	content: string;
	fileHash: string;
	crypto: RoomCrypto;
}) => {
	const encrypted = isEncryptedImageUrl(content);
	const [status, setStatus] = useState<Status>(
		encrypted ? { kind: "loading" } : { kind: "ready", src: content },
	);

	useEffect(() => {
		if (!encrypted) {
			return;
		}
		const controller = new AbortController();
		let objectUrl = "";
		// The fragment is ours (the file name); fetch never sends it anyway.
		const url = content.split("#")[0];

		// Without a hash the downloaded file can't be tied to this message.
		if (!fileHash) {
			setStatus({ kind: "mismatch" });
			return;
		}

		// Redirects are refused: the URL was checked to be on the API's host,
		// and following one could send the request anywhere.
		fetch(url, { signal: controller.signal, redirect: "error" })
			.then(async (res) => {
				if (!res.ok) {
					throw new Error(`status ${res.status}`);
				}
				const data = await readCapped(res, MAX_IMAGE_BYTES);
				const plain = new Uint8Array(await crypto.decryptFile(data, fileHash));
				if (controller.signal.aborted) {
					// Unmounted while decrypting: cleanup already ran, so a URL
					// made now would never be revoked.
					return;
				}
				objectUrl = URL.createObjectURL(
					new Blob([plain], { type: imageMimeType(imageName(content)) }),
				);
				setStatus({ kind: "ready", src: objectUrl });
			})
			.catch((e: unknown) => {
				if (!controller.signal.aborted) {
					setStatus({
						kind: e instanceof FileMismatchError ? "mismatch" : "error",
					});
				}
			});

		return () => {
			controller.abort();
			if (objectUrl) {
				URL.revokeObjectURL(objectUrl);
			}
		};
	}, [content, encrypted, fileHash, crypto]);

	const name = encrypted ? imageName(content) : "image";

	switch (status.kind) {
		case "loading":
			return <Note>decrypting image…</Note>;
		case "error":
			return <Note $danger>couldn't load {name}</Note>;
		case "mismatch":
			return (
				<Note $danger>
					{name} doesn't match its message (the file was swapped or the message
					has no file hash), so it isn't shown
				</Note>
			);
		case "ready":
			return (
				<a href={status.src} target="_blank" rel="noreferrer" title={name}>
					<Image src={status.src} alt={name} loading="lazy" />
				</a>
			);
	}
};

const Image = styled.img`
  display: block;
  max-width: min(100%, 22rem);
  max-height: 18rem;
  margin-top: 0.3rem;
  border: 1px solid ${({ theme }) => theme.colors.border};
  border-radius: 6px;
  object-fit: contain;
`;

const Note = styled.div<{ $danger?: boolean }>`
  color: ${({ theme, $danger }) =>
		$danger ? theme.colors.danger : theme.colors.text[300]};
  font-style: italic;
`;
