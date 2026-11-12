// Records one web demo with Playwright (see docs/demos/record-web.sh).
//
//   SCENE=chat INVITE_CODE=ABC123.key OUT=/out node scenes.mjs
//
// The page is the production build of clients/web served at localhost:3000;
// the other person in the room is sdk/go/examples/demo-bot.
//
// Frames are lossless screenshots taken about every FRAME_MS, written to
// OUT/<scene>-frames with an ffmpeg concat list holding each frame's real
// duration. (Playwright's own video is lossy VP8, whose noise changes every
// pixel of every frame and makes the GIF several times larger.)
import { mkdir, rm, writeFile } from "node:fs/promises";
import { join } from "node:path";
import { chromium } from "playwright";

const WEB = "http://localhost:3000/";
const { SCENE: scene, INVITE_CODE: invite = "", OUT: out = "." } = process.env;

// The bot's lines (sdk/go/examples/demo-bot/main.go).
const GREETING = "hey! glad you made it";
const BOT_REPLIES = [
	"yep, end-to-end encrypted. the server only ever sees ciphertext",
	"the room deletes itself when it expires. no accounts, no traces",
];

const pause = (page, ms) => page.waitForTimeout(ms);
const type = (page, text, delay = 55) => page.keyboard.type(text, { delay });

async function joinRoom(page, { slow }) {
	await page.getByRole("tab", { name: "Chat" }).click();
	await pause(page, slow ? 900 : 400);
	await page.locator("#invite-code").click();
	if (slow) {
		await type(page, invite, 18);
	} else {
		await page.locator("#invite-code").fill(invite);
	}
	await pause(page, 300);
	await page.getByRole("button", { name: /^Join/ }).click();
	await page.getByLabel("message").waitFor();
}

// sendMessage types a message and waits for the bot's answer.
async function sendMessage(page, text, reply) {
	await page.getByLabel("message").click();
	await type(page, text);
	await page.keyboard.press("Enter");
	if (reply) {
		await page.getByText(reply).waitFor({ timeout: 20_000 });
	}
}

const message = (page, text) =>
	page.getByRole("article").filter({ hasText: text }).first();

const scenes = {
	// Join with an invite code and chat.
	async chat(page) {
		await page.goto(WEB);
		await pause(page, 1500);
		await joinRoom(page, { slow: true });
		await page.getByText(GREETING).waitFor({ timeout: 20_000 });
		await pause(page, 1200);
		await sendMessage(page, "is this really end-to-end encrypted?", BOT_REPLIES[0]);
		await pause(page, 1500);
		await sendMessage(page, "nice. what happens when the room expires?", BOT_REPLIES[1]);
		await pause(page, 2500);
	},

	// React, reply and search.
	async reactions(page) {
		await page.goto(WEB);
		await pause(page, 800);
		await joinRoom(page, { slow: false });
		await page.getByText(GREETING).waitFor({ timeout: 20_000 });
		await pause(page, 1000);

		const greeting = message(page, GREETING);
		await greeting.hover();
		await pause(page, 600);
		await greeting.getByRole("button", { name: "react" }).click();
		await pause(page, 700);
		await greeting.getByRole("button", { name: "react with 🎉" }).click();
		await pause(page, 1200);

		await greeting.hover();
		await pause(page, 400);
		await greeting.getByRole("button", { name: "reply" }).click();
		await pause(page, 600);
		await type(page, "thanks! is this really encrypted?");
		await page.keyboard.press("Enter");
		await page.getByText(BOT_REPLIES[0]).waitFor({ timeout: 20_000 });
		await pause(page, 1500);

		await page.getByRole("button", { name: "search", exact: true }).click();
		await pause(page, 400);
		await type(page, "encrypted", 90);
		await pause(page, 2500);
		await page.keyboard.press("Escape");
		await pause(page, 1000);
	},

	// The terminal page and the other tabs.
	async tour(page) {
		await page.goto(WEB);
		await pause(page, 1500);
		await page.locator("input").first().click();
		for (const cmd of ["help", "themes set espresso", "about"]) {
			await type(page, cmd, 80);
			await pause(page, 300);
			await page.keyboard.press("Enter");
			await pause(page, 1800);
		}
		await type(page, "themes set visper", 80);
		await page.keyboard.press("Enter");
		await pause(page, 1000);
		for (const tab of ["Join room", "FAQ", "Settings", "Terminal"]) {
			await page.getByRole("tab", { name: tab }).click();
			await pause(page, 1600);
		}
	},
};

if (!scenes[scene]) {
	console.error(`unknown scene ${scene}; one of ${Object.keys(scenes).join(", ")}`);
	process.exit(2);
}
if (scene !== "tour" && !invite) {
	console.error("INVITE_CODE is required");
	process.exit(2);
}

const FRAME_MS = 100;

// recordFrames screenshots page until stop() is called and returns the
// concat list for ffmpeg.
function recordFrames(page, dir) {
	let recording = true;
	const frames = [];
	const done = (async () => {
		while (recording) {
			const started = Date.now();
			const file = `${String(frames.length).padStart(5, "0")}.png`;
			try {
				await page.screenshot({ path: join(dir, file) });
				frames.push({ file, at: started });
			} catch {
				// e.g. during a navigation; just skip this frame
			}
			await new Promise((r) => setTimeout(r, Math.max(0, FRAME_MS - (Date.now() - started))));
		}
	})();
	return async () => {
		recording = false;
		await done;
		const lines = ["ffconcat version 1.0"];
		frames.forEach((f, i) => {
			const next = frames[i + 1]?.at ?? f.at + FRAME_MS;
			lines.push(`file '${f.file}'`, `duration ${((next - f.at) / 1000).toFixed(3)}`);
		});
		// The last file is repeated so its duration is honoured.
		lines.push(`file '${frames.at(-1).file}'`);
		return `${lines.join("\n")}\n`;
	};
}

const framesDir = join(out, `${scene}-frames`);
await rm(framesDir, { recursive: true, force: true });
await mkdir(framesDir, { recursive: true });

const browser = await chromium.launch();
const context = await browser.newContext({
	viewport: { width: 1280, height: 720 },
	colorScheme: "dark",
});
const page = await context.newPage();
await page.goto("about:blank");
const stop = recordFrames(page, framesDir);
let failed = false;
try {
	await scenes[scene](page);
} catch (err) {
	failed = true;
	console.error(err);
}
await writeFile(join(framesDir, "frames.txt"), await stop());
if (failed) {
	await page.screenshot({ path: join(out, `${scene}-failure.png`) });
}
await context.close();
await browser.close();
process.exit(failed ? 1 : 0);
