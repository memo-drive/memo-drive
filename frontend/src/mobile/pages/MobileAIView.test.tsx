import { renderToString } from "react-dom/server";
import { MemoryRouter } from "react-router-dom";
import type { ReactElement } from "react";
import { describe, expect, it } from "vitest";
import type { ChatMessage } from "../../stores/chatStore";
import type { SourceChunk } from "../../types";
import "../../i18n";
import { MobileAIView } from "./MobileAIView";

describe("MobileAIView", () => {
  it("renders a full-screen mobile AI workspace with chat content and input", () => {
    const html = renderMobileAIView(
      <MobileAIView
        mode="rag"
        messages={[
          makeMessage({ role: "user", content: "总结一下我的会议纪要" }),
          makeMessage({
            id: "assistant-1",
            role: "assistant",
            content: "可以，我会基于你的文件内容整理。",
          }),
        ]}
        searchResults={[]}
        searchQuery=""
        busy={false}
        sending={false}
        loading={false}
        error=""
        onModeChange={() => undefined}
        onSubmit={() => undefined}
        onStop={() => undefined}
      />,
    );

    expect(html).toContain("移动 AI");
    expect(html).toContain("href=\"/m\"");
    expect(html).toContain("aria-label=\"返回\"");
    expect(html).toContain("arrow_back");
    expect(html).toContain("文件问答");
    expect(html).toContain("语义搜索");
    expect(html).toContain("总结一下我的会议纪要");
    expect(html).toContain("可以，我会基于你的文件内容整理。");
    expect(html).toContain("placeholder=\"问一个关于文件的问题...\"");
    expect(html).toContain("发送");
  });

  it("separates the scrollable conversation region from the fixed composer", () => {
    const html = renderMobileAIView(
      <MobileAIView
        mode="rag"
        messages={[makeMessage({ content: "哪些文件提到预算？" })]}
        searchResults={[]}
        searchQuery=""
        busy={false}
        sending={false}
        loading={false}
        error=""
        onModeChange={() => undefined}
        onSubmit={() => undefined}
        onStop={() => undefined}
      />,
    );

    expect(html).toContain("role=\"log\"");
    expect(html).toContain("aria-label=\"AI 对话内容\"");
    expect(html).toContain("aria-label=\"AI 输入区\"");
    expect(html).toContain("data-mobile-ai-scroll=\"content\"");
    expect(html).toContain("data-mobile-ai-composer=\"fixed\"");
  });

  it("renders semantic search results as tappable source targets", () => {
    const html = renderMobileAIView(
      <MobileAIView
        mode="search"
        messages={[]}
        searchResults={[
          makeSource({
            file_name: "预算.pdf",
            heading: "Q2 预算",
            snippet: "预算审批记录",
          }),
        ]}
        searchQuery="预算"
        busy={false}
        sending={false}
        loading={false}
        error=""
        onModeChange={() => undefined}
        onSubmit={() => undefined}
        onStop={() => undefined}
        onOpenSource={() => undefined}
      />,
    );

    expect(html).toContain("aria-label=\"打开来源文件\"");
    expect(html).toContain("预算.pdf");
    expect(html).toContain("Q2 预算");
    expect(html).toContain("预算审批记录");
  });

  it("renders the empty search state when the API returns null results", () => {
    const html = renderMobileAIView(
      <MobileAIView
        mode="search"
        messages={[]}
        searchResults={null}
        searchQuery="不存在的 pptx"
        busy={false}
        sending={false}
        loading={false}
        error=""
        onModeChange={() => undefined}
        onSubmit={() => undefined}
        onStop={() => undefined}
      />,
    );

    expect(html).toContain("没有找到相关片段");
  });
});

function renderMobileAIView(element: ReactElement) {
  return renderToString(<MemoryRouter>{element}</MemoryRouter>);
}

function makeMessage(overrides: Partial<ChatMessage>): ChatMessage {
  return {
    id: "message-1",
    role: "user",
    content: "",
    ...overrides,
  };
}

function makeSource(overrides: Partial<SourceChunk>): SourceChunk {
  return {
    id: "source-1",
    file_id: "file-1",
    file_name: "Memo.pdf",
    heading: "",
    chunk_index: 0,
    text: "source text",
    snippet: "",
    distance: 0.1,
    score: 0.88,
    ...overrides,
  };
}
