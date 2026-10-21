/**
 * Safe wrappers around `localStorage`. Access can throw (private mode,
 * blocked site data, SSR), so every call is guarded and failures are ignored.
 */
export function setToLS(key: string, value: string): void {
	try {
		window.localStorage.setItem(key, value);
	} catch {
		// Storage unavailable: preference simply isn't persisted.
	}
}

export function getFromLS(key: string): string | null {
	try {
		return window.localStorage.getItem(key) || null;
	} catch {
		return null;
	}
}
