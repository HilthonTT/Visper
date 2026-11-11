/**
 * Chat companions, mirroring the CLI's `embeds/waifus.json`. Images are
 * copies of the CLI's embedded PNGs, served from `public/companions/`.
 */
export type Companion = {
	id: number;
	title: string;
	description: string;
	src: string;
};

export const companions: Companion[] = [
	{
		id: 1,
		title: "Laughing Anime Waifu",
		description: "Zero Two cutely laughing, it's cute!",
		src: "/companions/waifu-1.png",
	},
	{
		id: 2,
		title: "Cool Waifu",
		description: "Futuristic waifu Zero Two",
		src: "/companions/waifu-2.png",
	},
];

export const DEFAULT_COMPANION_ID = 1;

export function getCompanion(id: number | null | undefined): Companion {
	return companions.find((c) => c.id === id) ?? companions[0];
}

export const DEFAULT_MOOD = "your waifu approves ♡";

/** Same lines the CLI companion uses after you send a message. */
const chatterMoods = [
	"nice one~",
	"they'll love that ♡",
	"keep it up!",
	"ooh, spicy",
	"so eloquent ✧",
];

const commandMoods: Record<string, string> = {
	help: "need a hand? ♡",
	clear: "squeaky clean ✧",
	themes: "ooh, new colours ✧",
	joinroom: "go say hi ♡",
	welcome: "welcome back~",
	whoami: "you're you, silly",
	echo: "echo… echo…",
};

/** The companion's reaction to a command typed at the terminal. */
export function moodForCommand(cmd: string, known: boolean, n: number): string {
	if (cmd === "") {
		return DEFAULT_MOOD;
	}
	if (!known) {
		return "that's not a command, silly";
	}
	return commandMoods[cmd] ?? chatterMoods[n % chatterMoods.length];
}
