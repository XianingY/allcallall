import { createContext, useContext } from "react";

export const ChatConnectionContext = createContext(false);

export const useChatConnected = () => useContext(ChatConnectionContext);
