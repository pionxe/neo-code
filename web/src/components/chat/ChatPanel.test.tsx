import { beforeEach, describe, expect, it, vi } from 'vitest'
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import ChatPanel from './ChatPanel'
import { useChatStore } from '@/stores/useChatStore'
import { useSessionStore } from '@/stores/useSessionStore'
import { useUIStore } from '@/stores/useUIStore'

let mockGatewayAPI: any = null

vi.mock('@/context/RuntimeProvider', () => ({
  useGatewayAPI: () => mockGatewayAPI,
}))

vi.mock('./MessageList', () => ({
  default: () => <div data-testid="message-list" />,
}))

vi.mock('./ChatInput', () => ({
  default: () => <div className="input-box"><textarea data-testid="chat-input" /></div>,
}))

vi.mock('./ModelSelector', () => ({
  default: () => <div data-testid="model-selector" />,
}))

vi.mock('./TodoStrip', () => ({
  default: () => <div data-testid="todo-strip" />,
}))

function draftPlanMessage(revision = 1) {
  return {
    id: `plan_msg_${revision}`,
    role: 'assistant',
    type: 'plan',
    content: '实现审批面板',
    timestamp: Date.now(),
    planData: {
      id: 'plan-1',
      revision,
      status: 'draft',
      spec: { goal: '实现审批面板', steps: ['补 RPC', '补 Web'] },
      summary: { goal: '实现审批面板', key_steps: ['补 RPC', '补 Web'] },
      created_at: '2026-05-20T00:00:00Z',
      updated_at: '2026-05-20T00:00:00Z',
    },
  }
}

describe('ChatPanel', () => {
  beforeEach(() => {
    mockGatewayAPI = {
      resolvePermission: vi.fn().mockResolvedValue(undefined),
      resolveUserQuestion: vi.fn().mockResolvedValue(undefined),
      approvePlan: vi.fn().mockResolvedValue({ payload: { plan_id: 'plan-1', revision: 1, status: 'approved' } }),
      run: vi.fn().mockResolvedValue({ session_id: 'session-1', run_id: 'run-1' }),
      bindStream: vi.fn().mockResolvedValue(undefined),
    }

    useUIStore.setState({
      sidebarOpen: true,
      changesPanelOpen: false,
      fileTreePanelOpen: false,
      toggleSidebar: vi.fn(),
      toggleChangesPanel: vi.fn(),
      toggleFileTreePanel: vi.fn(),
      showToast: vi.fn(),
    } as any)

    useSessionStore.setState({
      currentSessionId: 'session-1',
      currentProjectId: '',
      projects: [],
      loading: false,
      _switchAbort: null,
      _initialBindDone: false,
    } as any)

    useChatStore.setState({
      messages: [],
      isGenerating: false,
      isCompacting: false,
      compactMode: '',
      compactMessage: '',
      permissionRequests: [],
      pendingUserQuestion: null,
      agentMode: 'build',
      permissionMode: 'default',
    } as any)
  })

  it('does not auto-resolve permission requests in default mode', async () => {
    useChatStore.setState({
      permissionRequests: [{
        request_id: 'req-default',
        tool_call_id: 'tool-1',
        tool_name: 'filesystem_edit',
        tool_category: 'filesystem',
        action_type: 'write',
        operation: 'edit',
        target_type: 'file',
        target: 'foo.txt',
        decision: '',
        reason: 'needs approval',
      }],
    } as any)

    render(<ChatPanel />)

    expect(screen.getByText('权限请求')).toBeInTheDocument()
    await new Promise((resolve) => setTimeout(resolve, 20))
    expect(mockGatewayAPI.resolvePermission).not.toHaveBeenCalled()
  })

  it('auto-resolves permission requests once in build bypass mode', async () => {
    useChatStore.setState({
      permissionMode: 'bypass',
      permissionRequests: [{
        request_id: 'req-bypass',
        tool_call_id: 'tool-2',
        tool_name: 'filesystem_edit',
        tool_category: 'filesystem',
        action_type: 'write',
        operation: 'edit',
        target_type: 'file',
        target: 'bar.txt',
        decision: '',
        reason: 'needs approval',
      }],
    } as any)

    render(<ChatPanel />)

    await waitFor(() => {
      expect(mockGatewayAPI.resolvePermission).toHaveBeenCalledTimes(1)
    })
    expect(mockGatewayAPI.resolvePermission).toHaveBeenCalledWith({
      request_id: 'req-bypass',
      decision: 'allow_once',
    })
  })

  it('does not auto-resolve the same request more than once before it is removed', async () => {
    useChatStore.setState({
      permissionMode: 'bypass',
      permissionRequests: [{
        request_id: 'req-once',
        tool_call_id: 'tool-3',
        tool_name: 'filesystem_edit',
        tool_category: 'filesystem',
        action_type: 'write',
        operation: 'edit',
        target_type: 'file',
        target: 'baz.txt',
        decision: '',
        reason: 'needs approval',
      }],
    } as any)

    render(<ChatPanel />)

    await waitFor(() => {
      expect(mockGatewayAPI.resolvePermission).toHaveBeenCalledTimes(1)
    })

    await act(async () => {
      useChatStore.setState({
        permissionRequests: [{
          request_id: 'req-once',
          tool_call_id: 'tool-3',
          tool_name: 'filesystem_edit',
          tool_category: 'filesystem',
          action_type: 'write',
          operation: 'edit',
          target_type: 'file',
          target: 'baz.txt',
          decision: '',
          reason: 'needs approval',
        }],
      } as any)
    })

    await new Promise((resolve) => setTimeout(resolve, 20))
    expect(mockGatewayAPI.resolvePermission).toHaveBeenCalledTimes(1)
  })

  it('submits single choice ask_user with additional free text only', async () => {
    useChatStore.setState({
      pendingUserQuestion: {
        request_id: 'uq-single',
        title: 'Choose an option',
        description: 'Pick one',
        kind: 'single_choice',
        options: ['A', 'B', 'C'],
        allow_skip: false,
      },
    } as any)

    render(<ChatPanel />)

    fireEvent.change(screen.getByPlaceholderText('否，我有其他想法要告诉Neo-Code'), {
      target: { value: '我有其他方案：先做灰度' },
    })
    fireEvent.click(screen.getByRole('button', { name: /提交/ }))

    await waitFor(() => {
      expect(mockGatewayAPI.resolveUserQuestion).toHaveBeenCalledWith({
        request_id: 'uq-single',
        status: 'answered',
        values: undefined,
        message: '我有其他方案：先做灰度',
      })
    })
  })

  it('submits multi choice ask_user with selected values and additional free text', async () => {
    useChatStore.setState({
      pendingUserQuestion: {
        request_id: 'uq-multi',
        title: 'Choose options',
        description: 'Pick one or more',
        kind: 'multi_choice',
        options: ['A', 'B', 'C'],
        allow_skip: false,
      },
    } as any)

    render(<ChatPanel />)

    fireEvent.click(screen.getByRole('checkbox', { name: 'A' }))
    fireEvent.click(screen.getByRole('checkbox', { name: 'C' }))
    fireEvent.change(screen.getByPlaceholderText('否，我有其他想法要告诉Neo-Code'), {
      target: { value: 'C 放到后面，我建议先做 A' },
    })
    fireEvent.click(screen.getByRole('button', { name: /提交/ }))

    await waitFor(() => {
      expect(mockGatewayAPI.resolveUserQuestion).toHaveBeenCalledWith({
        request_id: 'uq-multi',
        status: 'answered',
        values: ['A', 'C'],
        message: 'C 放到后面，我建议先做 A',
      })
    })
  })

  it('renders option description tooltip icon for ask_user options', () => {
    useChatStore.setState({
      pendingUserQuestion: {
        request_id: 'uq-desc',
        title: 'Choose one',
        description: 'Pick one option',
        kind: 'single_choice',
        options: [
          { label: '选项 A', description: '先执行方案 A' },
          { label: '选项 B', description: '再执行方案 B' },
        ],
        allow_skip: false,
      },
    } as any)

    render(<ChatPanel />)

    const optionADescriptionBtn = screen.getByRole('button', { name: '查看选项说明：选项 A' })
    const optionBDescriptionBtn = screen.getByRole('button', { name: '查看选项说明：选项 B' })
    expect(optionADescriptionBtn).toBeInTheDocument()
    expect(optionBDescriptionBtn).toBeInTheDocument()

    fireEvent.click(optionADescriptionBtn)
    expect(screen.getByText('先执行方案 A')).toBeInTheDocument()
  })

  it('shows compact status panel above the normal chat input', () => {
    useChatStore.setState({
      isCompacting: true,
      compactMode: 'proactive',
      compactMessage: 'Context is near the limit. Auto-compacting...',
    } as any)

    render(<ChatPanel />)

    expect(screen.getByRole('status')).toHaveTextContent('Context is near the limit. Auto-compacting...')
    expect(screen.getByTestId('chat-input')).toBeInTheDocument()
  })

  it('keeps compact status visible while a permission request is shown', () => {
    useChatStore.setState({
      isCompacting: true,
      compactMode: 'reactive',
      compactMessage: 'Model reported context too long. Compacting and retrying...',
      permissionRequests: [{
        request_id: 'req-compact-permission',
        tool_call_id: 'tool-compact',
        tool_name: 'filesystem_edit',
        tool_category: 'filesystem',
        action_type: 'write',
        operation: 'edit',
        target_type: 'file',
        target: 'foo.txt',
        decision: '',
        reason: 'needs approval',
      }],
    } as any)

    render(<ChatPanel />)

    expect(screen.getByRole('status')).toHaveTextContent('Model reported context too long. Compacting and retrying...')
    expect(screen.getByText('权限请求')).toBeInTheDocument()
    expect(screen.queryByTestId('chat-input')).not.toBeInTheDocument()
  })

  it('keeps compact status visible while a user question is shown', () => {
    useChatStore.setState({
      isCompacting: true,
      compactMode: 'manual',
      compactMessage: 'Compacting context...',
      pendingUserQuestion: {
        request_id: 'ask-compact',
        question_id: 'question-compact',
        title: 'Need input',
        description: '',
        kind: 'text',
        required: true,
        allow_skip: false,
      },
    } as any)

    render(<ChatPanel />)

    expect(screen.getByRole('status')).toHaveTextContent('Compacting context...')
    expect(screen.getByText('Need input')).toBeInTheDocument()
    expect(screen.queryByTestId('chat-input')).not.toBeInTheDocument()
  })

  it('shows plan approval panel for latest draft plan', () => {
    useChatStore.setState({
      messages: [draftPlanMessage()],
    } as any)

    render(<ChatPanel />)

    expect(screen.getByText('计划审批')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /同意并以 bypass 执行/ })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /同意并以 default 执行/ })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /需要修改/ })).toBeInTheDocument()
  })

  it('approves plan with bypass mode and starts build run', async () => {
    useChatStore.setState({
      messages: [draftPlanMessage()],
      agentMode: 'plan',
      permissionMode: 'default',
    } as any)

    render(<ChatPanel />)
    fireEvent.click(screen.getByRole('button', { name: /同意并以 bypass 执行/ }))

    await waitFor(() => {
      expect(mockGatewayAPI.approvePlan).toHaveBeenCalledWith({
        session_id: 'session-1',
        plan_id: 'plan-1',
        revision: 1,
      })
    })
    await waitFor(() => {
      expect(mockGatewayAPI.run).toHaveBeenCalledWith({
        session_id: 'session-1',
        input_text: '按已批准计划执行',
        mode: 'build',
      })
    })
    expect(useChatStore.getState().agentMode).toBe('build')
    expect(useChatStore.getState().permissionMode).toBe('bypass')
  })

  it('approves plan with default mode and starts build run', async () => {
    useChatStore.setState({
      messages: [draftPlanMessage()],
      agentMode: 'plan',
      permissionMode: 'bypass',
    } as any)

    render(<ChatPanel />)
    fireEvent.click(screen.getByRole('button', { name: /同意并以 default 执行/ }))

    await waitFor(() => {
      expect(mockGatewayAPI.approvePlan).toHaveBeenCalledWith({
        session_id: 'session-1',
        plan_id: 'plan-1',
        revision: 1,
      })
    })
    expect(mockGatewayAPI.run).toHaveBeenCalledWith({
      session_id: 'session-1',
      input_text: '按已批准计划执行',
      mode: 'build',
    })
    expect(useChatStore.getState().agentMode).toBe('build')
    expect(useChatStore.getState().permissionMode).toBe('default')
  })

  it('rejects current plan revision and shows approval again for a new revision', async () => {
    useChatStore.setState({
      messages: [draftPlanMessage(1)],
    } as any)

    render(<ChatPanel />)
    fireEvent.click(screen.getByRole('button', { name: /需要修改/ }))

    await waitFor(() => {
      expect(screen.queryByText('计划审批')).not.toBeInTheDocument()
    })
    expect(mockGatewayAPI.approvePlan).not.toHaveBeenCalled()
    expect(screen.getByTestId('chat-input')).toHaveFocus()

    act(() => {
      useChatStore.setState({ messages: [draftPlanMessage(2)] } as any)
    })

    expect(await screen.findByText('计划审批')).toBeInTheDocument()
  })

  it('keeps plan approval panel when approve fails', async () => {
    mockGatewayAPI.approvePlan.mockRejectedValueOnce(new Error('revision mismatch'))
    useChatStore.setState({
      messages: [draftPlanMessage()],
    } as any)

    render(<ChatPanel />)
    fireEvent.click(screen.getByRole('button', { name: /同意并以 default 执行/ }))

    await waitFor(() => {
      expect(useUIStore.getState().showToast).toHaveBeenCalledWith('revision mismatch', 'error')
    })
    expect(mockGatewayAPI.run).not.toHaveBeenCalled()
    expect(screen.getByText('计划审批')).toBeInTheDocument()
  })

  it('shows retry execution panel when approved plan run fails', async () => {
    mockGatewayAPI.run.mockRejectedValueOnce(new Error('run failed'))
    useChatStore.setState({
      messages: [draftPlanMessage()],
      agentMode: 'plan',
      permissionMode: 'default',
    } as any)

    render(<ChatPanel />)
    fireEvent.click(screen.getByRole('button', { name: /同意并以 bypass 执行/ }))

    expect(await screen.findByRole('button', { name: /重试执行已批准计划/ })).toBeInTheDocument()
    expect(mockGatewayAPI.approvePlan).toHaveBeenCalledTimes(1)
    expect(mockGatewayAPI.run).toHaveBeenCalledTimes(1)
    expect(useUIStore.getState().showToast).toHaveBeenCalledWith('Failed to start approved plan run', 'error')
    expect(useChatStore.getState().permissionMode).toBe('bypass')
  })

  it('retries approved plan execution without approving again', async () => {
    mockGatewayAPI.run
      .mockRejectedValueOnce(new Error('run failed'))
      .mockResolvedValueOnce({ session_id: 'session-1', run_id: 'run-retry' })
    useChatStore.setState({
      messages: [draftPlanMessage()],
      agentMode: 'plan',
      permissionMode: 'default',
    } as any)

    render(<ChatPanel />)
    fireEvent.click(screen.getByRole('button', { name: /同意并以 default 执行/ }))
    fireEvent.click(await screen.findByRole('button', { name: /重试执行已批准计划/ }))

    await waitFor(() => {
      expect(mockGatewayAPI.run).toHaveBeenCalledTimes(2)
    })
    expect(mockGatewayAPI.approvePlan).toHaveBeenCalledTimes(1)
    expect(mockGatewayAPI.run).toHaveBeenLastCalledWith({
      session_id: 'session-1',
      input_text: '按已批准计划执行',
      mode: 'build',
    })
    expect(mockGatewayAPI.bindStream).toHaveBeenCalledWith({ session_id: 'session-1', channel: 'all' })
    await waitFor(() => {
      expect(screen.queryByRole('button', { name: /重试执行已批准计划/ })).not.toBeInTheDocument()
    })
  })
})
