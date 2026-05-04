import { useEffect, useRef, useState, type PointerEvent } from "react";
import { useAIChat } from "../../hooks/useAIChat";
import { AssistantPane } from "./AssistantPane";
import styles from "./AIFloatyBall.module.css";

const DRAG_THRESHOLD = 5;

export function AIFloatyBall() {
  const [expanded, setExpanded] = useState(false);
  const { stop } = useAIChat();

  const [pos, setPos] = useState(() => ({
    x: window.innerWidth - 80,
    y: window.innerHeight - 200,
  }));
  const [panelPos, setPanelPos] = useState(() => ({
    x: Math.max(window.innerWidth - 440, 20),
    y: 200,
  }));

  const dragging = useRef(false);
  const dragStart = useRef({ x: 0, y: 0 });
  const dragTarget = useRef<"ball" | "panel">("ball");
  const moved = useRef(false);
  const posRef = useRef(pos);
  const panelPosRef = useRef(panelPos);
  posRef.current = pos;
  panelPosRef.current = panelPos;

  function openPanel() {
    const ball = posRef.current;
    const pw = 400;
    const ph = 600;
    let px = ball.x - pw / 2 + 28;
    let py = ball.y - ph - 16;
    px = Math.max(12, Math.min(px, window.innerWidth - pw - 12));
    py = Math.max(12, Math.min(py, window.innerHeight - ph - 12));
    setPanelPos({ x: px, y: py });
    setExpanded(true);
  }

  function onPointerDown(event: PointerEvent, target: "ball" | "panel") {
    event.stopPropagation();
    (event.target as HTMLElement).setPointerCapture?.(event.pointerId);
    dragging.current = true;
    dragTarget.current = target;
    moved.current = false;
    dragStart.current = { x: event.clientX, y: event.clientY };
  }

  function onPointerMove(event: PointerEvent) {
    if (!dragging.current) return;
    const dx = event.clientX - dragStart.current.x;
    const dy = event.clientY - dragStart.current.y;
    if (Math.abs(dx) > DRAG_THRESHOLD || Math.abs(dy) > DRAG_THRESHOLD) {
      moved.current = true;
    }
    if (!moved.current) return;

    if (dragTarget.current === "ball") {
      const cur = posRef.current;
      setPos({
        x: Math.max(0, Math.min(cur.x + dx, window.innerWidth - 56)),
        y: Math.max(0, Math.min(cur.y + dy, window.innerHeight - 56)),
      });
    } else {
      const cur = panelPosRef.current;
      setPanelPos({
        x: Math.max(0, Math.min(cur.x + dx, window.innerWidth - 400)),
        y: Math.max(0, Math.min(cur.y + dy, window.innerHeight - 60)),
      });
    }
    dragStart.current = { x: event.clientX, y: event.clientY };
  }

  function onPointerUp() {
    if (!dragging.current) return;
    dragging.current = false;
    if (!moved.current && dragTarget.current === "ball") {
      openPanel();
    }
  }

  useEffect(() => {
    return () => {
      dragging.current = false;
      stop();
    };
  }, [stop]);

  return (
    <>
      {!expanded && (
        <div
          className={styles.ball}
          style={{ top: pos.y, left: pos.x }}
          onPointerDown={(event) => onPointerDown(event, "ball")}
          onPointerMove={onPointerMove}
          onPointerUp={onPointerUp}
          role="button"
          aria-label="打开 AI 助手"
        >
          <span className={styles.pulseRing} />
          <span className={`material-symbols-outlined ${styles.ballIcon}`}>
            psychiatry
          </span>
        </div>
      )}

      {expanded && (
        <div
          className={styles.panel}
          style={{ top: panelPos.y, left: panelPos.x }}
        >
          <AssistantPane
            floating
            onClose={() => setExpanded(false)}
            headerProps={{
              onPointerDown: (event: PointerEvent<HTMLDivElement>) =>
                onPointerDown(event, "panel"),
              onPointerMove,
              onPointerUp,
            }}
          />
        </div>
      )}
    </>
  );
}
