import { renderToString } from "react-dom/server";
import { expect, it, vi } from "vitest";
import type { PipelineTaskListItem } from "../../types";
import "../../i18n";
import {
  appendPipelineTaskPage,
  createLatestPipelineTaskPageLoader,
  PipelineTaskPanel,
  retryPipelineTaskAndRefresh,
  shouldPollPipelineTasks,
} from "./PipelineTaskPanel";

it("renders Task status filters and a quiet initial loading state", () => {
  const html = renderToString(<PipelineTaskPanel />);

  for (const label of ["全部", "等待处理", "正在处理", "处理失败", "处理完成"]) {
    expect(html).toContain(label);
  }
  expect(html).toContain("正在加载处理任务");
  expect(html).not.toContain('aria-live="assertive"');
});

it("polls only while an active Task is visible", () => {
  const active = makeTask("processing");
  const done = makeTask("done");

  expect(shouldPollPipelineTasks([active], "visible")).toBe(true);
  expect(shouldPollPipelineTasks([active], "hidden")).toBe(false);
  expect(shouldPollPipelineTasks([done], "visible")).toBe(false);
});

it("appends cursor pages without rendering duplicate Tasks", () => {
  const processing = makeTask("processing");
  const updated = { ...processing, progress: 75 };
  const failed = makeTask("failed");

  const merged = appendPipelineTaskPage([processing], [updated, failed]);

  expect(merged.map((item) => item.id)).toEqual([processing.id, failed.id]);
  expect(merged[0].progress).toBe(75);
});

it("ignores an older Task page after a newer request completes", async () => {
  let resolveOlder!: (value: ReturnType<typeof taskPage>) => void;
  let resolveNewer!: (value: ReturnType<typeof taskPage>) => void;
  const request = vi
    .fn()
    .mockImplementationOnce(() => new Promise((resolve) => { resolveOlder = resolve; }))
    .mockImplementationOnce(() => new Promise((resolve) => { resolveNewer = resolve; }));
  const loadLatest = createLatestPipelineTaskPageLoader(request);

  const older = loadLatest({ status: "failed" });
  const newer = loadLatest({ status: "done" });
  const newerPage = taskPage(makeTask("done"));
  resolveNewer(newerPage);
  await expect(newer).resolves.toEqual(newerPage);
  resolveOlder(taskPage(makeTask("failed")));
  await expect(older).resolves.toBeUndefined();
});

it("refreshes the Task list only after retry succeeds", async () => {
  const task = makeTask("failed");
  const retry = vi.fn().mockResolvedValue({ task: { id: "new-task" } });
  const refresh = vi.fn().mockResolvedValue(undefined);

  await retryPipelineTaskAndRefresh(task, retry, refresh);

  expect(retry).toHaveBeenCalledWith(task.id);
  expect(refresh).toHaveBeenCalledOnce();

  retry.mockRejectedValueOnce(new Error("queue full"));
  await expect(retryPipelineTaskAndRefresh(task, retry, refresh)).rejects.toThrow("queue full");
  expect(refresh).toHaveBeenCalledOnce();
});

function makeTask(status: PipelineTaskListItem["status"]): PipelineTaskListItem {
  return {
    id: `task-${status}`,
    file_id: "file-1",
    type: "pipeline",
    status,
    progress: status === "done" ? 100 : 30,
    retry_count: 0,
    created_at: "2026-08-12T04:00:00Z",
    updated_at: "2026-08-12T04:00:00Z",
    file: {
      id: "file-1",
      name: "notes.md",
      path: "/",
      size: 10,
      mime_type: "text/markdown",
      status,
    },
  };
}

function taskPage(item: PipelineTaskListItem) {
  return { items: [item], next_cursor: "", has_more: false };
}
