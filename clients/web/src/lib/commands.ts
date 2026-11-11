export type Command = {
	cmd: string;
	desc: string;
	tab: number;
	special?: boolean;
};

export const commands: Command[] = [
	{ cmd: "about", desc: "about Visper", tab: 8 },
	{ cmd: "clear", desc: "clear the terminal", tab: 8 },
	{ cmd: "echo", desc: "print out anything", tab: 9 },
	{ cmd: "env", desc: "print environment variables", tab: 10 },
	{ cmd: "setuserid", desc: "sets the user id", tab: 4, special: true },
	{
		cmd: "setjoincode",
		desc: "sets the room join code",
		tab: 2,
		special: true,
	},
	{
		cmd: "setsecurecode",
		desc: "sets the room secure code",
		tab: 0,
		special: true,
	},
	{
		cmd: "joinroom",
		desc: "joins room with the specified information",
		tab: 5,
		special: true,
	},
	{ cmd: "whoami", desc: "about current user", tab: 7 },
	{ cmd: "help", desc: "check available commands", tab: 9 },
	{ cmd: "history", desc: "view command history", tab: 6 },
	{ cmd: "pwd", desc: "print current working directory", tab: 10 },
	{ cmd: "welcome", desc: "display hero section", tab: 6 },
	{ cmd: "themes", desc: "check available themes", tab: 7 },
];

export const commandNames = commands.map(({ cmd }) => cmd);

export const isValidCommand = (name: string): boolean =>
	commandNames.includes(name);
