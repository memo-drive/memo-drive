import { renderToString } from "react-dom/server";
import { describe, expect, it } from "vitest";
import type { ChatMessage } from "../../stores/chatStore";
import "../../i18n";
import { MobileAIView } from "./MobileAIView";

describe("MobileAIView", () => {
  it("renders a full-screen mobile AI workspace with chat content and input", () => {
    const html = renderToString(
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
    expect(html).toContain("文件问答");
    expect(html).toContain("语义搜索");
    expect(html).toContain("总结一下我的会议纪要");
    expect(html).toContain("可以，我会基于你的文件内容整理。");
    expect(html).toContain("placeholder=\"问一个关于文件的问题...\"");
    expect(html).toContain("发送");
  });

  it("separates the scrollable conversation region from the fixed composer", () => {
    const html = renderToString(
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
});

function makeMessage(overrides: Partial<ChatMessage>): ChatMessage {
  return {
    id: "message-1",
    role: "user",
    content: "",
    ...overrides,
  };
}
