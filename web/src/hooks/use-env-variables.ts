import { useUser } from "@/contexts/user-context";

export const useEnvVariables = () => {
  const { userId, secureCode, joinCode } = useUser();

  return {
    USER: userId || "anonymous",
    USERID: userId || "anonymous",
    SECURECODE: secureCode || "",
    JOINCODE: joinCode || "",
    HOME: "/home/visper",
    PWD: "/home/visper",
  };
};
