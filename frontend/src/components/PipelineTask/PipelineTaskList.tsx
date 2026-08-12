import { useTranslation } from "react-i18next";
import type { PipelineTaskListItem, PipelineTaskStatus } from "../../types";
import { formatBytes } from "../../utils/formatBytes";

interface PipelineTaskListProps {
  items: PipelineTaskListItem[];
  onRetry: (task: PipelineTaskListItem) => void;
  retryingTaskID?: string;
}

export function PipelineTaskList({ items, onRetry, retryingTaskID = "" }: PipelineTaskListProps) {
  const { t } = useTranslation();
  if (items.length === 0) {
    return <p className="py-8 text-center text-sm text-zinc-500">{t("pipelineTask.empty")}</p>;
  }

  return (
    <ul role="list" aria-label={t("pipelineTask.title")} className="flex list-none flex-col gap-3 p-0">
      {items.map((item) => (
        <li key={item.id}>
          <article className="rounded-xl border border-zinc-200 p-4">
            <header className="flex items-start justify-between gap-4">
              <div className="min-w-0">
                <h3 className="truncate text-sm font-semibold">{item.file.name}</h3>
                <p className="mt-1 text-xs text-zinc-500">
                  {item.file.path} · {formatBytes(item.file.size)} · {new Date(item.created_at).toLocaleString()}
                </p>
              </div>
              <strong className="shrink-0 text-sm">{taskStatusLabel(t, item.status)}</strong>
            </header>

            <div className="mt-3 flex items-center gap-3">
              <progress
                value={Math.max(0, Math.min(100, item.progress))}
                max={100}
                aria-label={t("pipelineTask.progress", { name: item.file.name })}
                className="h-2 min-w-0 flex-1"
              />
              <span className="text-xs tabular-nums text-zinc-600">{item.progress}%</span>
            </div>

            {item.error ? <p className="mt-2 text-sm text-red-700">{item.error}</p> : null}

            <footer className="mt-3 flex items-center justify-between gap-3">
              <span className="text-xs text-zinc-500">
                {t("pipelineTask.retryCount", { count: item.retry_count })}
              </span>
              {item.status === "failed" ? (
                <button
                  type="button"
                  aria-label={t("pipelineTask.retryNamed", { name: item.file.name })}
                  disabled={retryingTaskID === item.id}
                  onClick={() => onRetry(item)}
                  className="rounded-lg border border-zinc-300 px-3 py-1.5 text-sm font-medium disabled:opacity-50"
                >
                  {t("pipelineTask.retry")}
                </button>
              ) : null}
            </footer>
          </article>
        </li>
      ))}
    </ul>
  );
}

function taskStatusLabel(
  t: (key: string) => string,
  status: PipelineTaskStatus | string,
): string {
  switch (status) {
    case "pending":
    case "processing":
    case "done":
    case "failed":
      return t(`pipelineTask.status.${status}`);
    default:
      return status;
  }
}
