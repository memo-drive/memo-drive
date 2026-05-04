import { create } from "zustand";

export type MessageType = "success" | "error" | "warning" | "info";
export type MessagePosition =
  | "top-left"
  | "top-center"
  | "top-right"
  | "bottom-left"
  | "bottom-center"
  | "bottom-right";

export interface MessageItem {
  id: string;
  type: MessageType;
  content: string;
  duration: number;
  onClose?: () => void;
}

interface MessageState {
  messages: MessageItem[];
  add: (item: MessageItem) => void;
  remove: (id: string) => void;
}

export const useMessageStore = create<MessageState>((set) => ({
  messages: [],
  add: (item) =>
    set((s) => ({ messages: [...s.messages, item] })),
  remove: (id) =>
    set((s) => ({ messages: s.messages.filter((m) => m.id !== id) })),
}));

let _id = 0;
function uid() {
  return `msg-${++_id}-${Date.now()}`;
}

function open(
  type: MessageType,
  content: string,
  duration = 3000,
  onClose?: () => void,
) {
  const id = uid();
  useMessageStore.getState().add({ id, type, content, duration, onClose });
  if (duration > 0) {
    setTimeout(() => {
      useMessageStore.getState().remove(id);
      onClose?.();
    }, duration);
  }
  return id;
}

export const message = {
  success(content: string, duration?: number, onClose?: () => void) {
    return open("success", content, duration, onClose);
  },
  error(content: string, duration?: number, onClose?: () => void) {
    return open("error", content, duration, onClose);
  },
  warning(content: string, duration?: number, onClose?: () => void) {
    return open("warning", content, duration, onClose);
  },
  info(content: string, duration?: number, onClose?: () => void) {
    return open("info", content, duration, onClose);
  },
  remove(id: string) {
    useMessageStore.getState().remove(id);
  },
  destroy() {
    useMessageStore.setState({ messages: [] });
  },
};
