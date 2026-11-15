import { useEffect, useState } from "react";
import styled from "styled-components";
import {
	imageMimeType,
	imageName,
	isEncryptedImageUrl,
} from "@/lib/room-crypto";
import { FileMismatchError, type RoomCrypto } from "@/lib/room-keys";

const MAX_IMAGE_BYTES = 100 << 20;

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

		fetch(url, { signal: controller.signal })
			.then(async (res) => {
				if (!res.ok) {
					throw new Error(`status ${res.status}`);
				}
				const data = new Uint8Array(await res.arrayBuffer());
				if (data.length > MAX_IMAGE_BYTES) {
					throw new Error("image too large");
				}
				const plain = new Uint8Array(await crypto.decryptFile(data, fileHash));
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
