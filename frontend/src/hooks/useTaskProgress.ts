import { useEffect, useState } from "react";
import { httpClient } from "../api/client";
import type { Task } from "../types";

export function useTaskProgress(taskId?: string) {
  const [task, setTask] = useState<Task | null>(null);

  useEffect(() => {
    if (!taskId) return;
    let cancelled = false;
    const tick = async () => {
      const next = await httpClient.get<Task>(`/tasks/${taskId}`);
      if (!cancelled) setTask(next);
      if (!cancelled && next.status !== "done" && next.status !== "failed") {
        window.setTimeout(tick, 1000);
      }
    };
    void tick();
    return () => {
      cancelled = true;
    };
  }, [taskId]);

  return task;
}
