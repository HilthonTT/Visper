import { createContext, useState, useContext, ReactNode } from "react";

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
  const [userId, setUserId] = useState<string>("");
  const [secureCode, setSecureCode] = useState<string>("");
  const [joinCode, setJoinCode] = useState<string>("");

  return (
    <UserContext.Provider
      value={{
        userId,
        secureCode,
        joinCode,
        setUserId,
        setSecureCode,
        setJoinCode,
      }}
    >
      {children}
    </UserContext.Provider>
  );
};

export const useUser = () => {
  const context = useContext(UserContext);
  if (!context) {
    throw new Error("useUser must be used within UserProvider");
  }
  return context;
};
