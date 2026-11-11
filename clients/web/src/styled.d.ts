import "styled-components";

declare module "styled-components" {
	export interface DefaultTheme {
		id: string;
		name: string;
		colors: {
			/** Page backdrop behind the terminal window. */
			body: string;
			/** Terminal window background. */
			surface: string;
			/** Raised elements: title bar, chips, cards. */
			surfaceAlt: string;
			border: string;
			scrollHandle: string;
			scrollHandleHover: string;
			primary: string;
			/** Text drawn on a `primary` background (active tab pill). */
			onPrimary: string;
			secondary: string;
			success: string;
			warning: string;
			danger: string;
			text: {
				/** Main text. */
				100: string;
				/** Secondary text. */
				200: string;
				/** Muted text: hints, separators, placeholders. */
				300: string;
			};
		};
	}
}
