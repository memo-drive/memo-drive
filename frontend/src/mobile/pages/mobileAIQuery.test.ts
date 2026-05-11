import { describe, expect, it, vi } from "vitest";
import { submitMobileAIQuery } from "./mobileAIQuery";

describe("mobileAIQuery", () => {
  it("sends trimmed queries to RAG chat in Q&A mode", async () => {
    const send = vi.fn().mockResolvedValue(undefined);
    const search = vi.fn().mockResolvedValue(undefined);

    await submitMobileAIQuery({
      mode: "rag",
      query: "  总结会议纪要  ",
      busy: false,
      send,
      search,
    });

    expect(send).toHaveBeenCalledWith("总结会议纪要");
    expect(search).not.toHaveBeenCalled();
  });

  it("runs semantic search in Search mode", async () => {
    const send = vi.fn().mockResolvedValue(undefined);
    const search = vi.fn().mockResolvedValue(undefined);

    await submitMobileAIQuery({
      mode: "search",
      query: " 找发票 ",
      busy: false,
      send,
      search,
    });

    expect(search).toHaveBeenCalledWith("找发票");
    expect(send).not.toHaveBeenCalled();
  });
});
