import { useEffect, useRef } from "react";
import { useMessageStore } from "./messageStore";
import type { MessagePosition, MessageItem } from "./messageStore";
import styles from "./Message.module.css";

const iconMap = {
  success: { char: "✓", cls: styles.iconSuccess },
  error: { char: "✕", cls: styles.iconError },
  warning: { char: "!", cls: styles.iconWarning },
  info: { char: "i", cls: styles.iconInfo },
};

const positionClass: Record<MessagePosition, string> = {
  "top-left": styles.topLeft,
  "top-center": styles.topCenter,
  "top-right": styles.topRight,
  "bottom-left": styles.bottomLeft,
  "bottom-center": styles.bottomCenter,
  "bottom-right": styles.bottomRight,
};

interface ToastItemProps {
  item: MessageItem;
  onRemove: (id: string) => void;
}

function ToastItem({ item, onRemove }: ToastItemProps) {
  const timerRef = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);

  useEffect(() => {
    if (item.duration > 0) {
      timerRef.current = setTimeout(() => {
        onRemove(item.id);
        item.onClose?.();
      }, item.duration);
    }
    return () => {
      if (timerRef.current) clearTimeout(timerRef.current);
    };
  }, [item, onRemove]);

  const icon = iconMap[item.type];

  return (
    <div className={styles.toast} role="alert">
      <span className={`${styles.icon} ${icon.cls}`}>{icon.char}</span>
      <span className={styles.content}>{item.content}</span>
      <button
        className={styles.closeBtn}
        onClick={() => {
          if (timerRef.current) clearTimeout(timerRef.current);
          onRemove(item.id);
          item.onClose?.();
        }}
        aria-label="关闭"
      >
        ×
      </button>
    </div>
  );
}

interface MessageContainerProps {
  position?: MessagePosition;
}

export function MessageContainer({
  position = "top-right",
}: MessageContainerProps) {
  const messages = useMessageStore((s) => s.messages);
  const remove = useMessageStore((s) => s.remove);

  if (messages.length === 0) return null;

  return (
    <div className={`${styles.container} ${positionClass[position]}`}>
      {messages.map((item) => (
        <ToastItem key={item.id} item={item} onRemove={remove} />
      ))}
    </div>
  );
}
