import { useRef, type MouseEvent } from "react";

export const MOBILE_LONG_PRESS_DELAY_MS = 420;

export function useMobileLongPress(
  onLongPress?: () => void,
  delay = MOBILE_LONG_PRESS_DELAY_MS,
) {
  const timerRef = useRef<number | null>(null);
  const firedRef = useRef(false);

  function clearTimer() {
    if (timerRef.current !== null) {
      window.clearTimeout(timerRef.current);
      timerRef.current = null;
    }
  }

  function start() {
    if (!onLongPress) return;
    firedRef.current = false;
    clearTimer();
    timerRef.current = window.setTimeout(() => {
      firedRef.current = true;
      timerRef.current = null;
      onLongPress();
    }, delay);
  }

  function cancel() {
    clearTimer();
  }

  function consumeClickAfterLongPress() {
    if (!firedRef.current) return false;
    firedRef.current = false;
    return true;
  }

  return {
    onPointerDown: start,
    onPointerUp: cancel,
    onPointerLeave: cancel,
    onPointerCancel: cancel,
    onContextMenu: (event: MouseEvent) => {
      if (!onLongPress) return;
      event.preventDefault();
      onLongPress();
    },
    consumeClickAfterLongPress,
  };
}
