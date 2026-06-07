import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { mapHistoryMessages, useSessionStore } from "./useSessionStore";
import { useChatStore } from "./useChatStore";
import { useGatewayStore } from "./useGatewayStore";
import { useRuntimeInsightStore } from "./useRuntimeInsightStore";
import { useUIStore } from "./useUIStore";

beforeEach(() => {
  useSessionStore.setState(
    (useSessionStore.getInitialState?.() ?? {
      projects: [],
      currentSessionId: "",
      currentProjectId: "",
      loading: false,
      _pendingNewSession: false,
    }) as any,
  );
  useChatStore.setState({
    messages: [],
    isGenerating: false,
    streamingMessageId: "",
    permissionRequests: [],
    tokenUsage: null,
    phase: "",
    stopReason: "",
  } as any);
  useGatewayStore.setState({
    connectionState: "disconnected",
    currentRunId: "",
    token: "",
    authenticated: false,
  } as any);
  useRuntimeInsightStore.getState().reset();
});

afterEach(() => {
  vi.restoreAllMocks();
});

describe("useSessionStore", () => {
  it("mapHistoryMessages skips internal system acceptance reminders", () => {
    const mapped = mapHistoryMessages([
      { role: "user", content: "start" },
      {
        role: "system",
        content: [
          "<acceptance_continue>",
          "<completion_blocked_reason>pending_todo</completion_blocked_reason>",
          "</acceptance_continue>",
        ].join(""),
      },
      { role: "assistant", content: "visible answer" },
    ]);

    expect(mapped.map((m) => m.content)).toEqual(["start", "visible answer"]);
    expect(
      mapped.every((m) => m.content.includes("acceptance_continue") === false),
    ).toBe(true);
  });

  it("mapHistoryMessages skips leaked assistant acceptance control text", () => {
    const mapped = mapHistoryMessages([
      {
        role: "assistant",
        content:
          "<acceptance_continue><todo_convergence></todo_convergence></acceptance_continue>",
      },
      { role: "assistant", content: "normal assistant text" },
      {
        role: "assistant",
        content:
          "prefix <completion_blocked_reason>pending_todo</completion_blocked_reason>",
      },
    ]);

    expect(mapped.map((m) => m.content)).toEqual([
      "normal assistant text",
      "prefix <completion_blocked_reason>pending_todo</completion_blocked_reason>",
    ]);
  });

  it("mapHistoryMessages keeps tool results that contain acceptance-like text", () => {
    const mapped = mapHistoryMessages([
      {
        role: "assistant",
        content: "",
        tool_calls: [
          {
            id: "call-xml",
            name: "filesystem_read_file",
            arguments: '{"path":"fixture.xml"}',
          },
        ],
      },
      {
        role: "tool",
        content:
          "<completion_blocked_reason>literal fixture</completion_blocked_reason>\n<todo_convergence />",
        tool_call_id: "call-xml",
      },
    ]);

    expect(mapped).toHaveLength(1);
    expect(mapped[0]).toMatchObject({
      role: "tool",
      type: "tool_call",
      toolCallId: "call-xml",
      toolResult:
        "<completion_blocked_reason>literal fixture</completion_blocked_reason>\n<todo_convergence />",
      toolStatus: "done",
    });
  });

  it("mapHistoryMessages keeps normal messages and merges tool results", () => {
    const mapped = mapHistoryMessages([
      { role: "user", content: "please inspect" },
      {
        role: "assistant",
        content: "calling tool",
        tool_calls: [
          {
            id: "call-1",
            name: "filesystem_read_file",
            arguments: '{"path":"README.md"}',
          },
        ],
      },
      { role: "tool", content: "file content", tool_call_id: "call-1" },
    ]);

    expect(mapped).toHaveLength(3);
    expect(mapped[0]).toMatchObject({
      role: "user",
      type: "text",
      content: "please inspect",
    });
    expect(mapped[1]).toMatchObject({
      role: "assistant",
      type: "text",
      content: "calling tool",
    });
    expect(mapped[2]).toMatchObject({
      role: "tool",
      type: "tool_call",
      toolName: "filesystem_read_file",
      toolCallId: "call-1",
      toolResult: "file content",
      toolStatus: "done",
    });
  });

  it("mapHistoryMessages converts image parts into user attachments", () => {
    const mapped = mapHistoryMessages([
      {
        role: "user",
        content: "[image]",
        parts: [
          { type: "text", text: "describe this" },
          { type: "image", media: { asset_id: "asset-1", mime_type: "image/png", file_name: "a.png" } },
        ],
      },
    ], "sess-1", "workspace-b");

    expect(mapped).toHaveLength(1);
    expect(mapped[0]).toMatchObject({
      role: "user",
      content: "describe this",
      attachments: [
        {
          id: "hist_att_0_0_asset-1",
          sessionId: "sess-1",
          workspaceHash: "workspace-b",
          assetId: "asset-1",
          mimeType: "image/png",
          name: "a.png",
        },
      ],
    });
  });

  it("switchSession restores current_plan as a plan message without duplicated rendered text", async () => {
    const mockBindStream = vi.fn().mockResolvedValue({});
    const mockLoadSession = vi.fn().mockResolvedValue({
      payload: {
        agent_mode: "plan",
        current_plan: {
          id: "plan-1",
          revision: 1,
          status: "draft",
          spec: {
            goal: "修复 web plan 展示",
            steps: ["发事件", "渲染卡片"],
            constraints: ["不显示 JSON"],
            open_questions: ["是否需要审批"],
          },
          summary: { goal: "修复 web plan 展示", key_steps: ["发事件"] },
          created_at: "2026-05-20T00:00:00Z",
          updated_at: "2026-05-20T00:00:00Z",
        },
        messages: [
          { role: "user", content: "先给计划" },
          {
            role: "assistant",
            content:
              "### 目标\n\n修复 web plan 展示\n\n### 实施步骤\n\n- 发事件\n- 渲染卡片",
          },
        ],
      },
    });
    const mockAPI = {
      bindStream: mockBindStream,
      loadSession: mockLoadSession,
      listSessionTodos: vi.fn().mockResolvedValue({ payload: {} }),
      getRuntimeSnapshot: vi.fn().mockResolvedValue({ payload: {} }),
    } as any;

    await useSessionStore.getState().switchSession("sess-plan", mockAPI);

    const messages = useChatStore.getState().messages;
    expect(messages).toHaveLength(2);
    expect(messages[0]).toMatchObject({ role: "user", content: "先给计划" });
    expect(messages[1]).toMatchObject({
      type: "plan",
      planData: { id: "plan-1" },
    });
    expect(useChatStore.getState().agentMode).toBe("plan");
  });

  it("switchSession keeps current_plan at the original rendered message position", async () => {
    const mockBindStream = vi.fn().mockResolvedValue({});
    const mockLoadSession = vi.fn().mockResolvedValue({
      payload: {
        agent_mode: "plan",
        current_plan: {
          id: "plan-middle",
          revision: 3,
          status: "draft",
          spec: {
            goal: "保持计划顺序",
            steps: ["替换原位置"],
          },
          summary: { goal: "保持计划顺序", key_steps: ["替换原位置"] },
          created_at: "2026-05-20T00:00:00Z",
          updated_at: "2026-05-20T00:00:00Z",
        },
        messages: [
          { role: "user", content: "先规划" },
          {
            role: "assistant",
            content: "### 目标\n\n保持计划顺序\n\n### 实施步骤\n\n- 替换原位置",
          },
          { role: "user", content: "继续讨论" },
          { role: "assistant", content: "后续回复" },
        ],
      },
    });
    const mockAPI = {
      bindStream: mockBindStream,
      loadSession: mockLoadSession,
      listSessionTodos: vi.fn().mockResolvedValue({ payload: {} }),
      getRuntimeSnapshot: vi.fn().mockResolvedValue({ payload: {} }),
    } as any;

    await useSessionStore.getState().switchSession("sess-plan-middle", mockAPI);

    const messages = useChatStore.getState().messages;
    expect(messages).toHaveLength(4);
    expect(messages.map((m) => m.content)).toEqual([
      "先规划",
      "保持计划顺序",
      "继续讨论",
      "后续回复",
    ]);
    expect(messages[1]).toMatchObject({
      type: "plan",
      planData: { id: "plan-middle", revision: 3 },
    });
    expect(messages.some((m) => m.content.includes("### 目标"))).toBe(false);
  });

  it("switchSession appends current_plan when no rendered plan text exists", async () => {
    const mockBindStream = vi.fn().mockResolvedValue({});
    const mockLoadSession = vi.fn().mockResolvedValue({
      payload: {
        agent_mode: "plan",
        current_plan: {
          id: "plan-append",
          revision: 1,
          status: "draft",
          spec: {
            goal: "追加计划卡片",
            steps: ["兼容缺失文本"],
          },
          summary: { goal: "追加计划卡片", key_steps: ["兼容缺失文本"] },
          created_at: "2026-05-20T00:00:00Z",
          updated_at: "2026-05-20T00:00:00Z",
        },
        messages: [
          { role: "user", content: "打开会话" },
          { role: "assistant", content: "普通回复" },
        ],
      },
    });
    const mockAPI = {
      bindStream: mockBindStream,
      loadSession: mockLoadSession,
      listSessionTodos: vi.fn().mockResolvedValue({ payload: {} }),
      getRuntimeSnapshot: vi.fn().mockResolvedValue({ payload: {} }),
    } as any;

    await useSessionStore.getState().switchSession("sess-plan-append", mockAPI);

    const messages = useChatStore.getState().messages;
    expect(messages).toHaveLength(3);
    expect(messages.map((m) => m.content)).toEqual([
      "打开会话",
      "普通回复",
      "追加计划卡片",
    ]);
    expect(messages[2]).toMatchObject({
      type: "plan",
      planData: { id: "plan-append", revision: 1 },
    });
  });

  it("switchSession still de-duplicates legacy rendered plan text", async () => {
    const mockBindStream = vi.fn().mockResolvedValue({});
    const mockLoadSession = vi.fn().mockResolvedValue({
      payload: {
        agent_mode: "plan",
        current_plan: {
          id: "plan-legacy",
          revision: 1,
          status: "draft",
          spec: {
            goal: "兼容旧计划格式",
            steps: ["保留旧前缀匹配"],
          },
          summary: { goal: "兼容旧计划格式", key_steps: ["保留旧前缀匹配"] },
          created_at: "2026-05-20T00:00:00Z",
          updated_at: "2026-05-20T00:00:00Z",
        },
        messages: [
          { role: "user", content: "打开旧会话" },
          {
            role: "assistant",
            content: "目标\n兼容旧计划格式\n\n实施步骤\n- 保留旧前缀匹配",
          },
        ],
      },
    });
    const mockAPI = {
      bindStream: mockBindStream,
      loadSession: mockLoadSession,
      listSessionTodos: vi.fn().mockResolvedValue({ payload: {} }),
      getRuntimeSnapshot: vi.fn().mockResolvedValue({ payload: {} }),
    } as any;

    await useSessionStore.getState().switchSession("sess-plan-legacy", mockAPI);

    const messages = useChatStore.getState().messages;
    expect(messages).toHaveLength(2);
    expect(messages[0]).toMatchObject({ role: "user", content: "打开旧会话" });
    expect(messages[1]).toMatchObject({
      type: "plan",
      planData: { id: "plan-legacy" },
    });
  });

  it("createSession clears messages and resets session state", () => {
    useChatStore.getState().addMessage({
      id: "1",
      role: "user",
      content: "hello",
      type: "text",
      timestamp: 1,
    });
    useSessionStore.setState({ currentSessionId: "sess-1" });

    useSessionStore.getState().createSession();

    expect(useChatStore.getState().messages).toHaveLength(0);
    expect(useSessionStore.getState().currentSessionId).toBe("");
    expect((useSessionStore.getState() as any)._pendingNewSession).toBe(true);
  });

  it("createSession is blocked while generating", () => {
    const showToast = vi.fn();
    useChatStore.setState({ isGenerating: true } as any);
    useUIStore.setState({ showToast } as any);
    useSessionStore.setState({
      currentSessionId: "sess-1",
      currentProjectId: "group-1",
    });

    useSessionStore.getState().createSession();

    expect(useSessionStore.getState().currentSessionId).toBe("sess-1");
    expect(useSessionStore.getState().currentProjectId).toBe("group-1");
    expect(showToast).toHaveBeenCalledWith(
      "Cannot start a new session while generating; stop the current run first.",
      "info",
    );
  });

  it("prepareNewChat also clears state and does not set temp id", () => {
    useSessionStore.setState({ currentSessionId: "sess-1" });
    useChatStore.getState().addMessage({
      id: "1",
      role: "user",
      content: "hello",
      type: "text",
      timestamp: 1,
    });

    useSessionStore.getState().prepareNewChat();

    expect(useChatStore.getState().messages).toHaveLength(0);
    expect(useSessionStore.getState().currentSessionId).toBe("");
    expect(useSessionStore.getState().currentProjectId).toBe("");
    expect((useSessionStore.getState() as any)._pendingNewSession).toBe(true);
  });

  it("prepareNewChat is blocked while generating", () => {
    const showToast = vi.fn();
    useChatStore.setState({ isGenerating: true } as any);
    useUIStore.setState({ showToast } as any);
    useSessionStore.setState({
      currentSessionId: "sess-1",
      currentProjectId: "group-1",
    });

    useSessionStore.getState().prepareNewChat();

    expect(useSessionStore.getState().currentSessionId).toBe("sess-1");
    expect(useSessionStore.getState().currentProjectId).toBe("group-1");
    expect(showToast).toHaveBeenCalledWith(
      "Cannot start a new session while generating; stop the current run first.",
      "info",
    );
  });

  it("initializeActiveSession binds stream for valid session id", async () => {
    const mockBindStream = vi.fn().mockResolvedValue({});
    const mockAPI = { bindStream: mockBindStream } as any;

    useSessionStore.setState({ currentSessionId: "sess-1" });
    await useSessionStore.getState().initializeActiveSession(mockAPI);

    expect(mockBindStream).toHaveBeenCalledWith({
      session_id: "sess-1",
      channel: "all",
    });
  });

  it("initializeActiveSession skips binding for empty session id", async () => {
    const mockBindStream = vi.fn().mockResolvedValue({});
    const mockAPI = { bindStream: mockBindStream } as any;

    useSessionStore.setState({ currentSessionId: "" });
    await useSessionStore.getState().initializeActiveSession(mockAPI);

    expect(mockBindStream).not.toHaveBeenCalled();
  });

  it("initializeActiveSession shows toast when bindStream fails", async () => {
    const showToast = vi.fn();
    const mockBindStream = vi.fn().mockRejectedValue(new Error("bind failed"));
    const mockAPI = { bindStream: mockBindStream } as any;
    useUIStore.setState({ showToast } as any);

    useSessionStore.setState({
      currentSessionId: "sess-1",
      _initialBindDone: false,
    } as any);
    await useSessionStore.getState().initializeActiveSession(mockAPI);

    expect(mockBindStream).toHaveBeenCalledWith({
      session_id: "sess-1",
      channel: "all",
    });
    expect(useSessionStore.getState()._initialBindDone).toBe(false);
    expect(showToast).toHaveBeenCalledWith(
      "Failed to bind event stream; real-time messages may not arrive.",
      "error",
    );
  });

  it("switchSession binds stream and loads session data", async () => {
    const setMessagesSpy = vi.spyOn(useChatStore.getState(), "setMessages");
    const addMessageSpy = vi.spyOn(useChatStore.getState(), "addMessage");
    const mockBindStream = vi.fn().mockResolvedValue({});
    const mockLoadSession = vi.fn().mockResolvedValue({
      payload: {
        messages: [{ role: "user", content: "hello", tool_calls: [] }],
      },
    });
    const mockAPI = {
      bindStream: mockBindStream,
      loadSession: mockLoadSession,
    } as any;

    await useSessionStore.getState().switchSession("sess-2", mockAPI);

    expect(mockBindStream).toHaveBeenCalledWith({
      session_id: "sess-2",
      channel: "all",
    });
    expect(setMessagesSpy).toHaveBeenCalledTimes(1);
    expect(addMessageSpy).not.toHaveBeenCalled();
    expect(useChatStore.getState().messages).toHaveLength(1);
    expect(useChatStore.getState().messages[0].role).toBe("user");
  });

  it("switchSession keeps transitioning true until loadSession finishes", async () => {
    const mockBindStream = vi.fn().mockResolvedValue({});
    let resolveLoad!: (value: any) => void;
    const mockLoadSession = vi.fn().mockImplementation(
      () =>
        new Promise((resolve) => {
          resolveLoad = resolve;
        }),
    );
    const mockAPI = {
      bindStream: mockBindStream,
      loadSession: mockLoadSession,
    } as any;

    const switchPromise = useSessionStore
      .getState()
      .switchSession("sess-2", mockAPI);
    await Promise.resolve();

    expect(useChatStore.getState().isTransitioning).toBe(true);

    resolveLoad({ payload: { messages: [] } });
    await switchPromise;

    expect(useChatStore.getState().isTransitioning).toBe(false);
  });

  it("resetForWorkspaceSwitch aborts in-flight switchSession and blocks stale writeback", async () => {
    const mockBindStream = vi.fn().mockResolvedValue({});
    let resolveLoad!: (value: any) => void;
    const mockLoadSession = vi.fn().mockImplementation(
      () =>
        new Promise((resolve) => {
          resolveLoad = resolve;
        }),
    );
    const mockAPI = {
      bindStream: mockBindStream,
      loadSession: mockLoadSession,
    } as any;

    const switchPromise = useSessionStore
      .getState()
      .switchSession("sess-old", mockAPI);
    await Promise.resolve();

    useSessionStore.getState().resetForWorkspaceSwitch();

    resolveLoad({
      payload: {
        messages: [
          { role: "assistant", content: "stale payload", tool_calls: [] },
        ],
        agent_mode: "plan",
      },
    });
    await switchPromise;

    expect(useChatStore.getState().messages).toHaveLength(0);
    expect(useChatStore.getState().agentMode).toBe("build");
  });

  it("switchSession applies only latest request when older request resolves later", async () => {
    const mockBindStream = vi.fn().mockResolvedValue({});
    let resolveLoadA!: (value: any) => void;
    let resolveLoadB!: (value: any) => void;
    const mockLoadSession = vi
      .fn()
      .mockImplementationOnce(
        () =>
          new Promise((resolve) => {
            resolveLoadA = resolve;
          }),
      )
      .mockImplementationOnce(
        () =>
          new Promise((resolve) => {
            resolveLoadB = resolve;
          }),
      );
    const mockAPI = {
      bindStream: mockBindStream,
      loadSession: mockLoadSession,
    } as any;

    const switchA = useSessionStore.getState().switchSession("sess-a", mockAPI);
    await Promise.resolve();
    const switchB = useSessionStore.getState().switchSession("sess-b", mockAPI);
    await Promise.resolve();

    resolveLoadB({
      payload: {
        messages: [
          { role: "assistant", content: "new payload", tool_calls: [] },
        ],
        agent_mode: "plan",
      },
    });
    await switchB;

    resolveLoadA({
      payload: {
        messages: [
          { role: "assistant", content: "old payload", tool_calls: [] },
        ],
        agent_mode: "build",
      },
    });
    await switchA;

    expect(useSessionStore.getState().currentSessionId).toBe("sess-b");
    expect(useChatStore.getState().messages).toHaveLength(1);
    expect(useChatStore.getState().messages[0].content).toBe("new payload");
    expect(useChatStore.getState().agentMode).toBe("plan");
  });

  it("fetchSessions auto-selects first session and binds stream", async () => {
    const setMessagesSpy = vi.spyOn(useChatStore.getState(), "setMessages");
    const addMessageSpy = vi.spyOn(useChatStore.getState(), "addMessage");
    const mockListSessions = vi.fn().mockResolvedValue({
      payload: {
        sessions: [
          {
            id: "sess-a",
            title: "Alpha",
            created_at: "2026-05-09T01:00:00Z",
            updated_at: "2026-05-09T02:00:00Z",
          },
        ],
      },
    });
    const mockBindStream = vi.fn().mockResolvedValue({});
    const mockLoadSession = vi.fn().mockResolvedValue({
      payload: {
        messages: [
          { role: "assistant", content: "loaded history", tool_calls: [] },
        ],
      },
    });
    const mockAPI = {
      listSessions: mockListSessions,
      bindStream: mockBindStream,
      loadSession: mockLoadSession,
    } as any;

    await useSessionStore.getState().fetchSessions(mockAPI);

    expect(useSessionStore.getState().currentSessionId).toBe("sess-a");
    expect(mockBindStream).toHaveBeenCalledWith({
      session_id: "sess-a",
      channel: "all",
    });
    expect(setMessagesSpy).toHaveBeenCalled();
    expect(addMessageSpy).not.toHaveBeenCalled();
    expect(useChatStore.getState().messages[0]).toMatchObject({
      role: "assistant",
      content: "loaded history",
    });
  });

  it("fetchSessions does not auto-select when current session is valid", async () => {
    const mockListSessions = vi.fn().mockResolvedValue({
      payload: {
        sessions: [
          {
            id: "sess-a",
            title: "Alpha",
            created_at: "2026-05-09T01:00:00Z",
            updated_at: "2026-05-09T02:00:00Z",
          },
        ],
      },
    });
    const mockBindStream = vi.fn().mockResolvedValue({});
    const mockAPI = {
      listSessions: mockListSessions,
      bindStream: mockBindStream,
    } as any;

    useSessionStore.setState({ currentSessionId: "sess-b" });
    await useSessionStore.getState().fetchSessions(mockAPI);

    expect(useSessionStore.getState().currentSessionId).toBe("sess-b");
    expect(mockBindStream).not.toHaveBeenCalled();
  });

  it("fetchSessions does not auto-select after explicit new session intent", async () => {
    const mockListSessions = vi.fn().mockResolvedValue({
      payload: {
        sessions: [
          {
            id: "sess-a",
            title: "Alpha",
            created_at: "2026-05-09T01:00:00Z",
            updated_at: "2026-05-09T02:00:00Z",
          },
        ],
      },
    });
    const mockBindStream = vi.fn().mockResolvedValue({});
    const mockLoadSession = vi.fn().mockResolvedValue({ payload: { messages: [] } });
    const mockAPI = {
      listSessions: mockListSessions,
      bindStream: mockBindStream,
      loadSession: mockLoadSession,
    } as any;

    useSessionStore.setState({ currentSessionId: "sess-old" } as any);
    useSessionStore.getState().createSession();
    await useSessionStore.getState().fetchSessions(mockAPI, true);

    expect(useSessionStore.getState().currentSessionId).toBe("");
    expect((useSessionStore.getState() as any)._pendingNewSession).toBe(true);
    expect(mockBindStream).not.toHaveBeenCalled();
    expect(mockLoadSession).not.toHaveBeenCalled();

    useSessionStore.getState().setCurrentSessionId("sess-new");
    expect(useSessionStore.getState().currentSessionId).toBe("sess-new");
    expect((useSessionStore.getState() as any)._pendingNewSession).toBe(false);
  });

  it("fetchSessions ignores stale late response from an older request", async () => {
    let resolveFirst!: (value: any) => void;
    let resolveSecond!: (value: any) => void;
    const mockListSessions = vi
      .fn()
      .mockImplementationOnce(
        () =>
          new Promise((resolve) => {
            resolveFirst = resolve;
          }),
      )
      .mockImplementationOnce(
        () =>
          new Promise((resolve) => {
            resolveSecond = resolve;
          }),
      );
    const mockAPI = {
      listSessions: mockListSessions,
      bindStream: vi.fn().mockResolvedValue({}),
      loadSession: vi.fn().mockResolvedValue({ payload: { messages: [] } }),
    } as any;

    useSessionStore.setState({ currentSessionId: "sess-keep" });

    const firstRequest = useSessionStore
      .getState()
      .fetchSessions(mockAPI, true);
    const secondRequest = useSessionStore
      .getState()
      .fetchSessions(mockAPI, true);

    resolveSecond({
      payload: {
        sessions: [
          {
            id: "sess-new",
            title: "New",
            created_at: "2026-05-10T01:00:00Z",
            updated_at: "2026-05-10T01:00:00Z",
          },
        ],
      },
    });
    await secondRequest;

    resolveFirst({
      payload: {
        sessions: [
          {
            id: "sess-old",
            title: "Old",
            created_at: "2026-05-09T01:00:00Z",
            updated_at: "2026-05-09T01:00:00Z",
          },
        ],
      },
    });
    await firstRequest;

    const sessions = useSessionStore
      .getState()
      .projects.flatMap((project) => project.sessions);
    expect(sessions.map((session) => session.id)).toEqual(["sess-new"]);
  });

  it("fetchSessions shows toast when auto bind or load fails", async () => {
    const showToast = vi.fn();
    useUIStore.setState({ showToast } as any);
    const mockListSessions = vi.fn().mockResolvedValue({
      payload: {
        sessions: [
          {
            id: "sess-a",
            title: "Alpha",
            created_at: "2026-05-09T01:00:00Z",
            updated_at: "2026-05-09T02:00:00Z",
          },
        ],
      },
    });
    const mockBindStream = vi.fn().mockRejectedValue(new Error("bind failed"));
    const mockAPI = {
      listSessions: mockListSessions,
      bindStream: mockBindStream,
    } as any;

    await useSessionStore.getState().fetchSessions(mockAPI);

    expect(useSessionStore.getState().currentSessionId).toBe("sess-a");
    expect(showToast).toHaveBeenCalledWith("Failed to load session", "error");
  });

  it("fetchSessions clears projects when listSessions fails", async () => {
    useSessionStore.setState({
      projects: [
        {
          id: "group",
          name: "Group",
          sessions: [
            { id: "sess-1", title: "A", time: "2026-05-10T00:00:00.000Z" },
          ],
        },
      ],
    } as any);
    const mockAPI = {
      listSessions: vi.fn().mockRejectedValue(new Error("list failed")),
    } as any;

    await useSessionStore.getState().fetchSessions(mockAPI, true);

    expect(useSessionStore.getState().projects).toEqual([]);
    expect(useSessionStore.getState().loading).toBe(false);
  });

  it("fetchSessions uses the newer of created_at/updated_at as display time", async () => {
    const mockListSessions = vi.fn().mockResolvedValue({
      payload: {
        sessions: [
          {
            id: "sess-a",
            title: "Alpha",
            created_at: "2026-05-09T09:30:00Z",
            updated_at: "2026-05-09T08:30:00Z",
          },
        ],
      },
    });
    const mockBindStream = vi.fn().mockResolvedValue({});
    const mockLoadSession = vi
      .fn()
      .mockResolvedValue({ payload: { messages: [] } });
    const mockAPI = {
      listSessions: mockListSessions,
      bindStream: mockBindStream,
      loadSession: mockLoadSession,
    } as any;

    await useSessionStore.getState().fetchSessions(mockAPI);

    const session = useSessionStore.getState().projects[0].sessions[0];
    expect(session.time).toBe("2026-05-09T09:30:00.000Z");
  });

  it("fetchSessions uses stable fallback time when created_at and updated_at are both invalid", async () => {
    const mockListSessions = vi.fn().mockResolvedValue({
      payload: {
        sessions: [
          {
            id: "sess-invalid-time",
            title: "InvalidTime",
            created_at: "not-a-date",
            updated_at: "",
          },
        ],
      },
    });
    const mockBindStream = vi.fn().mockResolvedValue({});
    const mockLoadSession = vi
      .fn()
      .mockResolvedValue({ payload: { messages: [] } });
    const mockAPI = {
      listSessions: mockListSessions,
      bindStream: mockBindStream,
      loadSession: mockLoadSession,
    } as any;

    await useSessionStore.getState().fetchSessions(mockAPI);

    const session = useSessionStore.getState().projects[0].sessions[0];
    expect(session.time).toBe("1970-01-01T00:00:00.000Z");
  });

  it("switchSession concurrently fetches todos and runtime snapshot", async () => {
    const mockBindStream = vi.fn().mockResolvedValue({});
    const mockLoadSession = vi.fn().mockResolvedValue({
      payload: {
        messages: [{ role: "user", content: "hello", tool_calls: [] }],
      },
    });
    const mockListSessionTodos = vi.fn().mockResolvedValue({
      payload: {
        items: [
          {
            id: "t1",
            content: "x",
            status: "open",
            required: true,
            revision: 1,
          },
        ],
        summary: {
          total: 1,
          required_total: 1,
          required_completed: 0,
          required_failed: 0,
          required_open: 1,
        },
      },
    });
    const mockGetRuntimeSnapshot = vi.fn().mockResolvedValue({ payload: {} });
    const mockAPI = {
      bindStream: mockBindStream,
      loadSession: mockLoadSession,
      listSessionTodos: mockListSessionTodos,
      getRuntimeSnapshot: mockGetRuntimeSnapshot,
    } as any;

    await useSessionStore.getState().switchSession("sess-2", mockAPI);

    expect(mockLoadSession).toHaveBeenCalledWith("sess-2");
    expect(mockListSessionTodos).toHaveBeenCalledWith("sess-2");
    expect(mockGetRuntimeSnapshot).toHaveBeenCalledWith("sess-2");

    const insightStore = useRuntimeInsightStore.getState();
    expect(insightStore.todoSnapshot?.items?.[0].id).toBe("t1");
  });

  it("removeSessionLocally prunes empty groups", () => {
    useSessionStore.setState({
      projects: [
        {
          id: "group-a",
          name: "A",
          sessions: [
            { id: "sess-1", title: "One", time: "2026-05-10T00:00:00.000Z" },
          ],
        },
        {
          id: "group-b",
          name: "B",
          sessions: [
            { id: "sess-2", title: "Two", time: "2026-05-10T00:00:00.000Z" },
            { id: "sess-3", title: "Three", time: "2026-05-10T00:00:00.000Z" },
          ],
        },
      ],
    } as any);

    useSessionStore.getState().removeSessionLocally("sess-1");
    useSessionStore.getState().removeSessionLocally("sess-2");

    expect(useSessionStore.getState().projects).toEqual([
      {
        id: "group-b",
        name: "B",
        sessions: [
          { id: "sess-3", title: "Three", time: "2026-05-10T00:00:00.000Z" },
        ],
      },
    ]);
  });
});
