import { renderToString } from "react-dom/server";
import { expect, it, vi } from "vitest";
import type { PipelineTaskListItem } from "../../types";
import "../../i18n";
import { PipelineTaskList } from "./PipelineTaskList";

it("renders failed pipeline Tasks with semantic progress and a named retry action", () => {
  const item: PipelineTaskListItem = {
    id: "task-1",
    file_id: "file-1",
    type: "pipeline",
    status: "failed",
    progress: 75,
    error: "embedding provider unavailable",
    retry_count: 2,
    retry_of_task_id: "task-0",
    created_at: "2026-08-12T04:00:00Z",
    updated_at: "2026-08-12T04:01:00Z",
    file: {
      id: "file-1",
      name: "report.pdf",
      path: "/Docs",
      size: 2048,
      mime_type: "application/pdf",
      status: "failed",
    },
  };

  const html = renderToString(
    <PipelineTaskList items={[item]} onRetry={vi.fn()} />,
  );

  expect(html).toContain("<ul");
  expect(html).toContain("<article");
  expect(html).toContain("report.pdf");
  expect(html).toContain("处理失败");
  expect(html).toContain("embedding provider unavailable");
  expect(html).toContain('<progress value="75" max="100"');
  expect(html).toContain('aria-label="重新处理 report.pdf"');
  expect(html).toContain("重试 2 次");
});
