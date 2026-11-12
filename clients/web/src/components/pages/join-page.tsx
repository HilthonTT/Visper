import { useEffect, useRef, useState } from "react";
import styled from "styled-components";
import { useShell } from "@/contexts/shell-context";
import { useUser } from "@/contexts/user-context";
import { notifySelfRoomInvite } from "@/lib/api";
import { Box, Button, Lead, PageContent, PageTitle } from "./page-styles";

type Status =
	| { kind: "idle" }
	| { kind: "sending" }
	| { kind: "error"; text: string }
	| { kind: "success"; text: string };

/**
 * Form version of `setdevicecode` + `setjoincode` + `setsecurecode` +
 * `setroomkey` + `joinroom`. Shares the same credentials as the terminal
 * commands. An invite link fills in everything but the device code.
 */
export const JoinPage = () => {
	const user = useUser();
	const { setMood } = useShell();
	const [status, setStatus] = useState<Status>({ kind: "idle" });
	const [focused, setFocused] = useState<string | null>(null);
	const controller = useRef<AbortController | null>(null);

	// Abort an in-flight invite if the page is left.
	useEffect(() => () => controller.current?.abort(), []);

	const fields = [
		{
			id: "deviceCode",
			label: "device code",
			hint: "from your CLI's settings page (press c there to copy)",
			value: user.deviceCode,
			set: user.setDeviceCode,
		},
		{
			id: "joinCode",
			label: "join code",
			hint: "the room code someone shared with you",
			value: user.joinCode,
			set: user.setJoinCode,
		},
		{
			id: "secureCode",
			label: "secure code",
			hint: "from the invite link",
			value: user.secureCode,
			set: user.setSecureCode,
		},
		{
			id: "roomKey",
			label: "room key",
			hint: "from the invite link; sealed to your CLI before sending",
			value: user.roomKey,
			set: user.setRoomKey,
		},
	] as const;

	const submit = (e: React.FormEvent) => {
		e.preventDefault();
		const missing = fields.find((f) => !f.value.trim());
		if (missing) {
			setStatus({ kind: "error", text: `Enter a ${missing.label} first.` });
			return;
		}

		controller.current?.abort();
		const c = new AbortController();
		controller.current = c;
		setStatus({ kind: "sending" });

		notifySelfRoomInvite(
			{
				deviceCode: user.deviceCode.trim(),
				joinCode: user.joinCode.trim(),
				secureCode: user.secureCode.trim(),
				roomKey: user.roomKey.trim(),
			},
			c.signal,
		)
			.then((data) => {
				setStatus({
					kind: "success",
					text: data.already_joined
						? `You're already in room ${data.room_id}.`
						: `Invite sent to ${data.username} for room ${data.room_id}. Check your CLI.`,
				});
				setMood("go say hi ♡");
			})
			.catch((error: unknown) => {
				if (c.signal.aborted) {
					return;
				}
				setStatus({
					kind: "error",
					text: error instanceof Error ? error.message : "Unknown error",
				});
				setMood("hmm, that didn't work…");
			});
	};

	const clear = () => {
		for (const f of fields) {
			f.set("");
		}
		setStatus({ kind: "idle" });
	};

	return (
		<PageContent>
			<PageTitle>Join a room</PageTitle>
			<Lead>
				Send a room invite to your CLI. The room key is encrypted to your CLI
				before it leaves this page, so the server can't read it.
			</Lead>

			<Form
				onSubmit={submit}
				onKeyDown={(e) => {
					if (e.key === "Escape") {
						clear();
					}
				}}
			>
				{fields.map((f) => (
					<label key={f.id} htmlFor={f.id}>
						<FieldLabel>{f.label}</FieldLabel>
						<Box
							$active={focused === f.id}
							$tone={
								status.kind === "error" && !f.value.trim()
									? "danger"
									: undefined
							}
						>
							<Prompt>›</Prompt>
							<Input
								id={f.id}
								type="text"
								autoComplete="off"
								spellCheck={false}
								placeholder={f.hint}
								value={f.value}
								onFocus={() => setFocused(f.id)}
								onBlur={() => setFocused(null)}
								onChange={(e) => {
									f.set(e.target.value);
									if (status.kind === "error") {
										setStatus({ kind: "idle" });
									}
								}}
							/>
						</Box>
					</label>
				))}

				<Actions>
					<Button type="submit" disabled={status.kind === "sending"}>
						Send invite ↵
					</Button>
					<StatusText $kind={status.kind} role="status">
						{status.kind === "sending" && "Sending…"}
						{status.kind === "error" && `✕ ${status.text}`}
						{status.kind === "success" && `✓ ${status.text}`}
						{status.kind === "idle" && "press enter to send"}
					</StatusText>
				</Actions>
			</Form>
		</PageContent>
	);
};

const Form = styled.form`
  display: flex;
  flex-direction: column;
  gap: 0.9rem;

  label {
    display: block;
  }
`;

const FieldLabel = styled.div`
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
  margin-top: 0.4rem;
`;

const StatusText = styled.span<{ $kind: Status["kind"] }>`
  color: ${({ theme, $kind }) =>
		({
			idle: theme.colors.text[300],
			sending: theme.colors.primary,
			error: theme.colors.danger,
			success: theme.colors.success,
		})[$kind]};
`;
