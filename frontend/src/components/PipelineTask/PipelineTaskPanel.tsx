import { useCallback, useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { listPipelineTasks, retryPipelineTask } from "../../api/taskApi";
import type {
  PipelineTaskListItem,
  PipelineTaskListPage,
  PipelineTaskStatus,
} from "../../types";
import { message } from "../base";
import { PipelineTaskList } from "./PipelineTaskList";

type TaskStatusFilter = "all" | PipelineTaskStatus;

const FILTERS: TaskStatusFilter[] = ["all", "pending", "processing", "failed", "done"];

export function shouldPollPipelineTasks(
  items: PipelineTaskListItem[],
  visibility: DocumentVisibilityState,
): boolean {
  return visibility === "visible" && items.some(
    (item) => item.status === "pending" || item.status === "processing",
  );
}

export function appendPipelineTaskPage(
  current: PipelineTaskListItem[],
  incoming: PipelineTaskListItem[],
): PipelineTaskListItem[] {
  const incomingByID = new Map(incoming.map((item) => [item.id, item]));
  const merged = current.map((item) => incomingByID.get(item.id) ?? item);
  const seen = new Set(current.map((item) => item.id));
  for (const item of incoming) {
    if (!seen.has(item.id)) {
      seen.add(item.id);
      merged.push(item);
    }
  }
  return merged;
}

export async function retryPipelineTaskAndRefresh(
  task: PipelineTaskListItem,
  retry: (taskID: string) => Promise<unknown>,
  refresh: () => Promise<void>,
): Promise<void> {
  await retry(task.id);
  await refresh();
}

export function createLatestPipelineTaskPageLoader(
  request: typeof listPipelineTasks = listPipelineTasks,
) {
  let latestRequest = 0;
  return async (
    ...args: Parameters<typeof listPipelineTasks>
  ): Promise<PipelineTaskListPage | undefined> => {
    const requestID = ++latestRequest;
    try {
      const page = await request(...args);
      return requestID === latestRequest ? page : undefined;
    } catch (error) {
      if (requestID !== latestRequest) return undefined;
      throw error;
    }
  };
}

export function PipelineTaskPanel() {
  const { t } = useTranslation();
  const [status, setStatus] = useState<TaskStatusFilter>("all");
  const [items, setItems] = useState<PipelineTaskListItem[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadingMore, setLoadingMore] = useState(false);
  const [nextCursor, setNextCursor] = useState("");
  const [hasMore, setHasMore] = useState(false);
  const [loadError, setLoadError] = useState("");
  const [retryingTaskID, setRetryingTaskID] = useState("");
  const [loadLatestPage] = useState(() => createLatestPipelineTaskPageLoader());

  const loadPage = useCallback(async (
    cursor: string,
    replace: boolean,
    background = false,
  ) => {
    if (replace && !background) setLoading(true);
    if (!replace) setLoadingMore(true);
    try {
      const page = await loadLatestPage({
        status: status === "all" ? undefined : status,
        cursor: cursor || undefined,
        limit: 25,
      }, { background });
      if (!page) return;
      setItems((current) => replace ? page.items : appendPipelineTaskPage(current, page.items));
      setNextCursor(page.next_cursor);
      setHasMore(page.has_more);
      setLoadError("");
    } catch (error) {
      if (!background) {
        setLoadError(error instanceof Error ? error.message : t("pipelineTask.loadFailed"));
      }
    } finally {
      if (replace && !background) setLoading(false);
      if (!replace) setLoadingMore(false);
    }
  }, [loadLatestPage, status, t]);

  useEffect(() => {
    void loadPage("", true);
  }, [loadPage]);

  useEffect(() => {
    if (!items.some((item) => item.status === "pending" || item.status === "processing")) return;
    const poll = () => {
      if (shouldPollPipelineTasks(items, document.visibilityState)) {
        void loadPage("", true, true);
      }
    };
    const interval = window.setInterval(poll, 3000);
    document.addEventListener("visibilitychange", poll);
    return () => {
      window.clearInterval(interval);
      document.removeEventListener("visibilitychange", poll);
    };
  }, [items, loadPage]);

  function selectStatus(filter: TaskStatusFilter) {
    if (filter === status) return;
    setItems([]);
    setLoadError("");
    setLoading(true);
    setStatus(filter);
  }

  async function handleRetry(task: PipelineTaskListItem) {
    if (retryingTaskID) return;
    setRetryingTaskID(task.id);
    try {
      await retryPipelineTaskAndRefresh(
        task,
        retryPipelineTask,
        () => loadPage("", true),
      );
      message.success(t("pipelineTask.retrySuccess"));
    } catch (error) {
      message.error(error instanceof Error ? error.message : t("pipelineTask.retryFailed"));
    } finally {
      setRetryingTaskID("");
    }
  }

  return (
    <section aria-labelledby="pipeline-task-title" className="flex min-h-0 flex-col gap-3">
      <h2 id="pipeline-task-title" className="text-lg font-semibold">
        {t("pipelineTask.title")}
      </h2>
      <div className="flex flex-wrap gap-2" aria-label={t("pipelineTask.filterLabel")}>
        {FILTERS.map((filter) => (
          <button
            key={filter}
            type="button"
            aria-pressed={status === filter}
            onClick={() => selectStatus(filter)}
            className="rounded-full border border-zinc-300 px-3 py-1.5 text-sm"
          >
            {t(`pipelineTask.filter.${filter}`)}
          </button>
        ))}
      </div>
      {loading ? (
        <p className="py-8 text-center text-sm text-zinc-500">{t("pipelineTask.loading")}</p>
      ) : loadError ? (
        <div className="rounded-xl border border-red-200 p-4 text-sm text-red-700">
          <p>{loadError}</p>
          <button type="button" className="mt-2 underline" onClick={() => void loadPage("", true)}>
            {t("pipelineTask.retryList")}
          </button>
        </div>
      ) : (
        <>
          <PipelineTaskList items={items} onRetry={(task) => void handleRetry(task)} retryingTaskID={retryingTaskID} />
          {hasMore ? (
            <button
              type="button"
              disabled={loadingMore}
              onClick={() => void loadPage(nextCursor, false)}
              className="self-center rounded-lg border border-zinc-300 px-4 py-2 text-sm disabled:opacity-50"
            >
              {t(loadingMore ? "pipelineTask.loadingMore" : "pipelineTask.loadMore")}
            </button>
          ) : null}
        </>
      )}
    </section>
  );
}
