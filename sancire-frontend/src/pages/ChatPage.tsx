import { useParams } from "react-router-dom";
import { ChatView } from "@/components/chat-view";
export default function ChatPage() {
  const { id = "" } = useParams();
  return <ChatView conversationId={id} />;
}
