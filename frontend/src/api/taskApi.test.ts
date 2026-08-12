import { afterEach, expect, it, vi } from "vitest";
import { listPipelineTasks, retryPipelineTask } from "./taskApi";

afterEach(() => {
  vi.unstubAllGlobals();
});

it("lists a filtered pipeline Task cursor page", async () => {
  const fetchMock = vi.fn().mockResolvedValue(
    new Response(JSON.stringify({ items: [], next_cursor: "", has_more: false }), {
      status: 200,
      headers: { "Content-Type": "application/json" },
    }),
  );
  vi.stubGlobal("fetch", fetchMock);

  await listPipelineTasks({
    status: "failed",
    fileID: "file / one",
    cursor: "next page",
    limit: 25,
  });

  expect(fetchMock).toHaveBeenCalledWith(
    "/api/tasks?status=failed&file_id=file%20%2F%20one&cursor=next%20page&limit=25",
    expect.any(Object),
  );
});

it("retries an encoded pipeline Task ID", async () => {
  const fetchMock = vi.fn().mockResolvedValue(
    new Response(JSON.stringify({ task: { id: "retry-1" } }), {
      status: 201,
      headers: { "Content-Type": "application/json" },
    }),
  );
  vi.stubGlobal("fetch", fetchMock);

  await retryPipelineTask("failed / one");

  expect(fetchMock).toHaveBeenCalledWith(
    "/api/tasks/failed%20%2F%20one/retry",
    expect.objectContaining({ method: "POST", body: "{}" }),
  );
});

it("deprioritizes background Task polling", async () => {
  const fetchMock = vi.fn().mockResolvedValue(
    new Response(JSON.stringify({ items: [], next_cursor: "", has_more: false }), {
      status: 200,
      headers: { "Content-Type": "application/json" },
    }),
  );
  vi.stubGlobal("fetch", fetchMock);

  await listPipelineTasks({}, { background: true });

  expect(fetchMock).toHaveBeenCalledWith(
    "/api/tasks",
    expect.objectContaining({ priority: "low" }),
  );
});
