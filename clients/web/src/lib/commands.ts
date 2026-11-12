export type Command = {
	cmd: string;
	desc: string;
	/** Room commands, grouped separately in `help`. */
	special?: boolean;
};

export const commands: Command[] = [
	{ cmd: "about", desc: "about Visper" },
	{ cmd: "clear", desc: "clear the terminal" },
	{ cmd: "echo", desc: "print out anything" },
	{ cmd: "env", desc: "print environment variables" },
	{
		cmd: "setdevicecode",
		desc: "sets your CLI's device code (from its settings page)",
		special: true,
	},
	{
		cmd: "setjoincode",
		desc: "sets the room join code",
		special: true,
	},
	{
		cmd: "setsecurecode",
		desc: "sets the room secure code",
		special: true,
	},
	{
		cmd: "setroomkey",
		desc: "sets the room key (from the invite link)",
		special: true,
	},
	{
		cmd: "joinroom",
		desc: "joins room with the specified information",
		special: true,
	},
	{ cmd: "whoami", desc: "about current user" },
	{ cmd: "help", desc: "check available commands" },
	{ cmd: "history", desc: "view command history" },
	{ cmd: "pwd", desc: "print current working directory" },
	{ cmd: "welcome", desc: "display hero section" },
	{ cmd: "themes", desc: "check available themes" },
];

export const commandNames = commands.map(({ cmd }) => cmd);

export const isValidCommand = (name: string): boolean =>
	commandNames.includes(name);
