import {
	createContext,
	type ReactNode,
	useCallback,
	useContext,
	useEffect,
	useMemo,
	useState,
} from "react";
import { DEFAULT_COMPANION_ID, DEFAULT_MOOD } from "@/lib/companions";
import { getFromLS, setToLS } from "@/lib/storage";

/** Top-level pages, in tab order (same order as the CLI's header). */
export const pages = [
	{ id: "terminal", key: "t", label: "Terminal" },
	{ id: "join", key: "j", label: "Join room" },
	{ id: "faq", key: "f", label: "FAQ" },
	{ id: "settings", key: "s", label: "Settings" },
] as const;

export type PageId = (typeof pages)[number]["id"];

type ShellContextType = {
	page: PageId;
	setPage: (page: PageId) => void;
	companionId: number;
	setCompanionId: (id: number) => void;
	companionHidden: boolean;
	toggleCompanion: () => void;
	mood: string;
	setMood: (mood: string) => void;
};

const COMPANION_KEY = "visper-companion";
const COMPANION_HIDDEN_KEY = "visper-companion-hidden";

const ShellContext = createContext<ShellContextType | undefined>(undefined);

export const ShellProvider = ({
	initialPage = "terminal",
	children,
}: {
	initialPage?: PageId;
	children: ReactNode;
}) => {
	const [page, setPage] = useState<PageId>(initialPage);
	const [companionId, setCompanionIdState] = useState(DEFAULT_COMPANION_ID);
	const [companionHidden, setCompanionHidden] = useState(false);
	const [mood, setMood] = useState(DEFAULT_MOOD);

	// Restore preferences after mount: storage isn't available during SSR.
	useEffect(() => {
		const stored = Number(getFromLS(COMPANION_KEY));
		if (Number.isInteger(stored) && stored > 0) {
			setCompanionIdState(stored);
		}
		setCompanionHidden(getFromLS(COMPANION_HIDDEN_KEY) === "1");
	}, []);

	const setCompanionId = useCallback((id: number) => {
		setToLS(COMPANION_KEY, String(id));
		setCompanionIdState(id);
		setMood("hi again ♡");
	}, []);

	const toggleCompanion = useCallback(() => {
		setCompanionHidden((hidden) => {
			setToLS(COMPANION_HIDDEN_KEY, hidden ? "0" : "1");
			return !hidden;
		});
	}, []);

	const value = useMemo(
		() => ({
			page,
			setPage,
			companionId,
			setCompanionId,
			companionHidden,
			toggleCompanion,
			mood,
			setMood,
		}),
		[page, companionId, setCompanionId, companionHidden, toggleCompanion, mood],
	);

	return (
		<ShellContext.Provider value={value}>{children}</ShellContext.Provider>
	);
};

export const useShell = () => {
	const context = useContext(ShellContext);
	if (!context) {
		throw new Error("useShell must be used within ShellProvider");
	}
	return context;
};
