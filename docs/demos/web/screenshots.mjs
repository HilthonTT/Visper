// Takes the web client screenshots in docs/assets/screenshots (see
// docs/demos/record-web.sh, scene "screenshots").
//
//   INVITE_CODE=ABC123.key OUT=/repo/docs/assets/screenshots node screenshots.mjs
//
// The page is the production build of clients/web served at localhost:3000.
// INVITE_CODE is a room of sdk/go/examples/demo-bot, for the key
// verification shot.
import { join } from "node:path";
import { chromium } from "playwright";

const WEB = "http://localhost:3000/";
const { INVITE_CODE: invite = "", OUT: out = "." } = process.env;

const pause = (page, ms) => page.waitForTimeout(ms);

// run types a command into the web terminal.
async function run(page, cmd) {
	await page.locator("input").first().click();
	await page.keyboard.type(cmd);
	await page.keyboard.press("Enter");
	await pause(page, 500);
}

const browser = await chromium.launch();

async function newPage(viewport) {
	const context = await browser.newContext({
		viewport,
		deviceScaleFactor: 2,
		colorScheme: "dark",
	});
	const page = await context.newPage();
	await page.goto(WEB);
	await page.getByTestId("welcome").waitFor();
	await pause(page, 1200); // fonts and the companion
	return page;
}

const shot = (page, name, opts = {}) =>
	page.screenshot({ path: join(out, `${name}.png`), ...opts });

let failed = false;
try {
	const desktop = { width: 1440, height: 900 };

	let page = await newPage(desktop);
	await shot(page, "web-terminal");
	await run(page, "help");
	await shot(page, "web-help");
	for (const [tab, name] of [
		["Join room", "web-join"],
		["Settings", "web-settings"],
		["FAQ", "web-faq"],
	]) {
		await page.getByRole("tab", { name: tab }).click();
		await pause(page, 800);
		await shot(page, name);
	}
	await page.context().close();

	page = await newPage(desktop);
	await run(page, "themes set light");
	await run(page, "clear");
	await run(page, "welcome");
	await pause(page, 600);
	await shot(page, "web-light");
	await page.context().close();

	page = await newPage({ width: 390, height: 844 });
	await shot(page, "web-mobile");
	await page.context().close();

	// Key verification: compare and verify the other member's key.
	if (invite) {
		page = await newPage(desktop);
		await page.getByRole("tab", { name: "Chat" }).click();
		await page.locator("#invite-code").fill(invite);
		await page.locator("#chat-username").fill("alice");
		await page.getByRole("button", { name: /^Join/ }).click();
		const member = page
			.getByLabel("Members")
			.locator('button[title^="compare keys with"]');
		await member.waitFor({ timeout: 20_000 });
		await pause(page, 1500); // the members' key cards
		await member.click();
		await page
			.getByLabel("Verify keys")
			.getByRole("button", { name: "mark verified" })
			.click();
		await pause(page, 600);
		await shot(page, "web-keys");
		await page.context().close();
	}
} catch (err) {
	failed = true;
	console.error(err);
}
await browser.close();
process.exit(failed ? 1 : 0);
