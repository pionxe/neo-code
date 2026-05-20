import { describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen } from "@testing-library/react";
import MessageItem from "./MessageItem";

vi.mock("./ToolCallCard", () => ({ default: () => <div>tool-card</div> }));
vi.mock("./AcceptanceMessage", () => ({
  default: () => <div>acceptance-card</div>,
}));
vi.mock("./CodeBlock", () => ({
  default: ({ code }: { code: string }) => <pre>{code}</pre>,
}));
vi.mock("./MarkdownContent", () => ({
  default: ({ content }: { content: string }) => <span>{content}</span>,
}));
vi.mock("@/context/RuntimeProvider", () => ({ useGatewayAPI: () => null }));

describe("MessageItem", () => {
  it("renders system message", () => {
    render(
      <MessageItem
        message={
          {
            id: "s1",
            role: "assistant",
            type: "system",
            content: "sys",
            timestamp: 1,
          } as any
        }
      />,
    );
    expect(screen.getByText("sys")).toBeInTheDocument();
  });

  it("renders welcome message", () => {
    render(
      <MessageItem
        message={
          {
            id: "w1",
            role: "assistant",
            type: "welcome",
            content: "hello",
            timestamp: 1,
          } as any
        }
      />,
    );
    expect(screen.getByText("hello")).toBeInTheDocument();
  });

  it("renders thinking message and toggles details", () => {
    render(
      <MessageItem
        message={
          {
            id: "t1",
            role: "assistant",
            type: "thinking",
            content: "reasoning",
            timestamp: 1,
            streaming: false,
            thinkingData: { collapsed: true },
          } as any
        }
      />,
    );
    fireEvent.click(screen.getByText("AI 思考过程"));
    expect(screen.getByText("reasoning")).toBeInTheDocument();
  });

  it("renders tool and acceptance delegates", () => {
    const { rerender } = render(
      <MessageItem
        message={
          {
            id: "m1",
            role: "tool",
            type: "tool_call",
            content: "",
            timestamp: 1,
          } as any
        }
      />,
    );
    expect(screen.getByText("tool-card")).toBeInTheDocument();
    rerender(
      <MessageItem
        message={
          {
            id: "m3",
            role: "assistant",
            type: "acceptance",
            content: "",
            timestamp: 1,
          } as any
        }
      />,
    );
    expect(screen.getByText("acceptance-card")).toBeInTheDocument();
  });

  it("renders plan message card", () => {
    render(
      <MessageItem
        message={
          {
            id: "p1",
            role: "assistant",
            type: "plan",
            content: "",
            timestamp: 1,
            planData: {
              id: "plan-1",
              revision: 2,
              status: "draft",
              spec: {
                goal: "修复计划展示",
                steps: ["发事件"],
                constraints: ["不显示 JSON"],
                open_questions: ["是否需要审批"],
              },
              summary: { goal: "修复计划展示", key_steps: ["发事件"] },
              created_at: "2026-05-20T00:00:00Z",
              updated_at: "2026-05-20T00:00:00Z",
            },
          } as any
        }
      />,
    );
    expect(screen.getByText("计划")).toBeInTheDocument();
    expect(screen.getByText("修复计划展示")).toBeInTheDocument();
    expect(screen.getByText("发事件")).toBeInTheDocument();
    expect(screen.getByText("不显示 JSON")).toBeInTheDocument();
    expect(screen.getByText("是否需要审批")).toBeInTheDocument();
  });

  it("renders code and plain assistant messages", () => {
    const { rerender } = render(
      <MessageItem
        message={
          {
            id: "c1",
            role: "assistant",
            type: "code",
            content: "const a=1",
            timestamp: 1,
          } as any
        }
      />,
    );
    expect(screen.getByText("const a=1")).toBeInTheDocument();
    rerender(
      <MessageItem
        message={
          {
            id: "a1",
            role: "assistant",
            type: "text",
            content: "answer",
            timestamp: 1,
          } as any
        }
      />,
    );
    expect(screen.getByText("answer")).toBeInTheDocument();
  });
});
