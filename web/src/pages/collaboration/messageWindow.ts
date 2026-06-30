import type { Message } from "@/api/collaboration";

export const MESSAGE_RENDER_WINDOW = 160;

export function windowMessages(messages: Message[], limit = MESSAGE_RENDER_WINDOW) {
  if (messages.length <= limit) return { visible: messages, hiddenCount: 0 };
  return { visible: messages.slice(messages.length - limit), hiddenCount: messages.length - limit };
}
