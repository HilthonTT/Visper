import { useUser } from "@/contexts/user-context";
import { Wrapper } from "../styles/output-styled";
import { useEffect, useState, useMemo } from "react";

type Status = "loading" | "success" | "error" | "idle";

type RoomInfo = {
  roomId?: string;
  roomName?: string;
};

export const JoinRoom = () => {
  const { userId, joinCode, secureCode } = useUser();

  const initialValues = useMemo(
    () => ({
      userId,
      joinCode,
      secureCode,
    }),
    [],
  );

  const [status, setStatus] = useState<Status>("idle");
  const [message, setMessage] = useState("");
  const [_, setRoomInfo] = useState<RoomInfo>({});

  useEffect(() => {
    const sendNotification = async () => {
      if (!initialValues.userId) {
        setStatus("error");
        setMessage("Error: User ID not set. Use 'setuserid <your-id>' first.");
        return;
      }

      if (!initialValues.joinCode) {
        setStatus("error");
        setMessage("Error: Join code not set. Use 'setjoincode <code>' first.");
        return;
      }

      if (!initialValues.secureCode) {
        setStatus("error");
        setMessage(
          "Error: Secure code not set. Use 'setsecurecode <code>' first.",
        );
        return;
      }

      setStatus("loading");
      setMessage("Sending notification to your devices...");

      try {
        const backendURL = import.meta.env.VITE_BACKEND_API_URL as string;
        const finalURL = `${backendURL}/api/v1/users/notifications/self-room-invite?join_code=${encodeURIComponent(initialValues.joinCode)}&secure_code=${encodeURIComponent(initialValues.secureCode)}`;
        const response = await fetch(finalURL, {
          method: "POST",
          headers: {
            "Content-Type": "application/json",
          },
          body: JSON.stringify({
            user_id: initialValues.userId,
          }),
        });

        if (!response.ok) {
          const contentType = response.headers.get("content-type");
          if (contentType && contentType.includes("application/json")) {
            const data = await response.json();
            setStatus("error");
            setMessage(
              `Error: ${data.message || "Failed to send notification"}`,
            );
          } else {
            setStatus("error");
            setMessage(
              `Error: Server returned ${response.status} - ${response.statusText}`,
            );
          }
          return;
        }

        const data = await response.json();

        if (data.already_joined) {
          setStatus("success");
          setMessage(`You are already a member of "${data.room_name}"`);
          setRoomInfo({
            roomId: data.room_id,
            roomName: data.room_name,
          });
          return;
        }

        setStatus("success");
        setMessage(
          `Notification sent successfully!\n` +
            `Room: ${data.room_name}\n` +
            `User: ${data.username}\n\n` +
            `Check your CLI for the room invite notification.`,
        );

        setRoomInfo({
          roomId: data.room_id,
          roomName: data.room_name,
        });
      } catch (error) {
        setStatus("error");
        setMessage(
          `Error: Failed to connect to server.\n${error instanceof Error ? error.message : "Unknown error"}`,
        );
      }
    };

    sendNotification();
  }, []);

  return (
    <Wrapper>
      {status === "loading" && (
        <div style={{ color: "#FFD700" }}>{message}</div>
      )}
      {status === "error" && <div style={{ color: "#FF6B6B" }}>{message}</div>}
      {status === "success" && (
        <div style={{ color: "#51CF66" }}>
          {message.split("\n").map((line, i) => (
            <div key={i}>{line || "\u00A0"}</div>
          ))}
        </div>
      )}
    </Wrapper>
  );
};
