import {
	createContext,
	type ReactNode,
	useContext,
	useMemo,
	useState,
} from "react";

type UserContextType = {
	userId: string;
	secureCode: string;
	joinCode: string;
	setUserId: (id: string) => void;
	setSecureCode: (code: string) => void;
	setJoinCode: (code: string) => void;
};

const UserContext = createContext<UserContextType | undefined>(undefined);

export const UserProvider = ({ children }: { children: ReactNode }) => {
	const [userId, setUserId] = useState("");
	const [secureCode, setSecureCode] = useState("");
	const [joinCode, setJoinCode] = useState("");

	const value = useMemo(
		() => ({
			userId,
			secureCode,
			joinCode,
			setUserId,
			setSecureCode,
			setJoinCode,
		}),
		[userId, secureCode, joinCode],
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
