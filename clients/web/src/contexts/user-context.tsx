import {
	createContext,
	type ReactNode,
	useContext,
	useMemo,
	useState,
} from "react";

/**
 * What an invite to your CLI needs. Kept in memory only: the room key unlocks
 * the room, so it is never written to storage.
 */
type UserContextType = {
	/** The device code from the CLI's settings page (user id + public key). */
	deviceCode: string;
	secureCode: string;
	joinCode: string;
	/** The room's encryption key, from the invite link's `#key=` fragment. */
	roomKey: string;
	setDeviceCode: (code: string) => void;
	setSecureCode: (code: string) => void;
	setJoinCode: (code: string) => void;
	setRoomKey: (key: string) => void;
};

const UserContext = createContext<UserContextType | undefined>(undefined);

export const UserProvider = ({ children }: { children: ReactNode }) => {
	const [deviceCode, setDeviceCode] = useState("");
	const [secureCode, setSecureCode] = useState("");
	const [joinCode, setJoinCode] = useState("");
	const [roomKey, setRoomKey] = useState("");

	const value = useMemo(
		() => ({
			deviceCode,
			secureCode,
			joinCode,
			roomKey,
			setDeviceCode,
			setSecureCode,
			setJoinCode,
			setRoomKey,
		}),
		[deviceCode, secureCode, joinCode, roomKey],
	);

	return <UserContext.Provider value={value}>{children}</UserContext.Provider>;
};

export const useUser = () => {
	const context = useContext(UserContext);
	if (!context) {
		throw new Error("useUser must be used within UserProvider");
	}
	return context;
};
