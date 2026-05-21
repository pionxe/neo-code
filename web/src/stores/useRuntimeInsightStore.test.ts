import { describe, it, expect, beforeEach } from 'vitest'
import { useRuntimeInsightStore } from './useRuntimeInsightStore'

beforeEach(() => {
  useRuntimeInsightStore.getState().reset()
})

describe('useRuntimeInsightStore', () => {
  it('calculates budget usage ratio when prompt budget is available', () => {
    useRuntimeInsightStore.getState().setBudgetChecked({
      attempt_seq: 1,
      request_hash: 'hash-1',
      action: 'allow',
      estimated_input_tokens: 50,
      prompt_budget: 100,
    })

    expect(useRuntimeInsightStore.getState().budgetUsageRatio).toBe(0.5)
  })

  it('uses null budget usage ratio when prompt budget is zero', () => {
    useRuntimeInsightStore.getState().setBudgetChecked({
      attempt_seq: 1,
      request_hash: 'hash-1',
      action: 'allow',
      estimated_input_tokens: 50,
      prompt_budget: 0,
    })

    expect(useRuntimeInsightStore.getState().budgetUsageRatio).toBeNull()
  })

  it('stores final verification outcomes', () => {
    const store = useRuntimeInsightStore.getState()

    store.completeVerification({ stop_reason: 'accepted' })
    expect(useRuntimeInsightStore.getState().verificationCompleted?.stop_reason).toBe('accepted')

    store.failVerification({ stop_reason: 'error', error_class: 'TestError' })
    expect(useRuntimeInsightStore.getState().verificationFailed?.error_class).toBe('TestError')
    expect(useRuntimeInsightStore.getState().verificationCompleted).toBeNull()
  })

  it('clears a stale failed terminal state when verification later completes', () => {
    const store = useRuntimeInsightStore.getState()

    store.failVerification({ stop_reason: 'error', error_class: 'TestError' })
    store.completeVerification({ stop_reason: 'accepted' })

    const state = useRuntimeInsightStore.getState()
    expect(state.verificationCompleted?.stop_reason).toBe('accepted')
    expect(state.verificationFailed).toBeNull()
  })

  it('clears a stale completed terminal state when verification later fails', () => {
    const store = useRuntimeInsightStore.getState()

    store.completeVerification({ stop_reason: 'accepted' })
    store.failVerification({ stop_reason: 'error', error_class: 'TestError' })

    const state = useRuntimeInsightStore.getState()
    expect(state.verificationFailed?.error_class).toBe('TestError')
    expect(state.verificationCompleted).toBeNull()
  })

  it('resets all insight state', () => {
    const store = useRuntimeInsightStore.getState()
    store.setAcceptanceDecision({ status: 'accepted', user_visible_summary: 'done' })
    store.completeVerification({ stop_reason: 'accepted' })
    store.setTodoSnapshot({ summary: { total: 1, required_total: 1, required_completed: 1, required_failed: 0, required_open: 0 } })

    store.reset()

    expect(useRuntimeInsightStore.getState().acceptanceDecision).toBeNull()
    expect(useRuntimeInsightStore.getState().verificationCompleted).toBeNull()
    expect(useRuntimeInsightStore.getState().todoSnapshot).toBeNull()
  })

  it('setTodoSnapshot clears any stale todoConflict on a valid update', () => {
    const store = useRuntimeInsightStore.getState()
    store.setTodoConflict({ action: 'todo_conflict', reason: 'todo_not_found' })
    expect(useRuntimeInsightStore.getState().todoConflict?.reason).toBe('todo_not_found')

    store.setTodoSnapshot({
      items: [{ id: 'a', content: 'task', status: 'pending', required: true, revision: 1 }],
      summary: { total: 1, required_total: 1, required_completed: 0, required_failed: 0, required_open: 1 },
    })

    expect(useRuntimeInsightStore.getState().todoConflict).toBeNull()
    expect(useRuntimeInsightStore.getState().todoSnapshot?.items?.[0].id).toBe('a')
  })

  it('setTodoSnapshot with empty items clears current snapshot items and conflict while preserving history', () => {
    const store = useRuntimeInsightStore.getState()
    store.setTodoSnapshot({
      items: [{ id: 'a', content: 'task a', status: 'in_progress', required: true, revision: 1 }],
      summary: { total: 1, required_total: 1, required_completed: 0, required_failed: 0, required_open: 1 },
    })
    store.setTodoConflict({ action: 'todo_conflict', reason: 'todo_not_found' })

    store.setTodoSnapshot({
      items: [],
      summary: { total: 0, required_total: 0, required_completed: 0, required_failed: 0, required_open: 0 },
    })

    const state = useRuntimeInsightStore.getState()
    expect(state.todoSnapshot?.items).toEqual([])
    expect(state.todoHistory.a).toBeDefined()
    expect(state.todoConflict).toBeNull()
  })

  it('applyTodoSnapshot updates snapshot but does not clear conflict', () => {
    const store = useRuntimeInsightStore.getState()
    store.setTodoConflict({ action: 'todo_conflict', reason: 'revision_conflict' })
    expect(useRuntimeInsightStore.getState().todoConflict?.reason).toBe('revision_conflict')

    store.applyTodoSnapshot({
      items: [{ id: 'b', content: 'task b', status: 'pending', required: true, revision: 2 }],
      summary: { total: 1, required_total: 1, required_completed: 0, required_failed: 0, required_open: 1 },
    })

    const state = useRuntimeInsightStore.getState()
    expect(state.todoSnapshot?.items?.[0].id).toBe('b')
    expect(state.todoConflict?.reason).toBe('revision_conflict')
  })

  it('applyTodoSnapshot with empty items clears current snapshot items but preserves conflict and history', () => {
    const store = useRuntimeInsightStore.getState()
    store.setTodoSnapshot({
      items: [{ id: 'a', content: 'task a', status: 'in_progress', required: true, revision: 1 }],
    })
    store.setTodoConflict({ action: 'todo_conflict', reason: 'todo_not_found' })

    store.applyTodoSnapshot({ items: [] })

    const state = useRuntimeInsightStore.getState()
    expect(state.todoSnapshot?.items).toEqual([])
    expect(state.todoConflict?.reason).toBe('todo_not_found')
    expect(state.todoHistory.a).toBeDefined()
  })

  it('applyTodoSnapshot can clear stale conflict on reset while preserving history', () => {
    const store = useRuntimeInsightStore.getState()
    store.setTodoSnapshot({
      items: [{ id: 'a', content: 'task a', status: 'in_progress', required: true, revision: 1 }],
    })
    store.setTodoConflict({ action: 'todo_conflict', reason: 'todo_not_found' })

    store.applyTodoSnapshot({ items: [] }, { clearConflict: true })

    const state = useRuntimeInsightStore.getState()
    expect(state.todoSnapshot?.items).toEqual([])
    expect(state.todoConflict).toBeNull()
    expect(state.todoHistory.a).toBeDefined()
  })

  it('setTodoSnapshot accumulates todoHistory across replacements', () => {
    const store = useRuntimeInsightStore.getState()
    store.setTodoSnapshot({
      items: [{ id: 'a', content: 'task a', status: 'pending', required: true, revision: 1 }],
    })
    store.setTodoSnapshot({
      items: [{ id: 'b', content: 'task b', status: 'in_progress', required: true, revision: 1 }],
    })

    const state = useRuntimeInsightStore.getState()
    expect(Object.keys(state.todoHistory).sort()).toEqual(['a', 'b'])
    expect(state.todoSnapshot?.items?.map((i) => i.id)).toEqual(['b'])
    expect(state.todoHistory.a.firstSeenAt).toBeLessThanOrEqual(state.todoHistory.b.firstSeenAt)
  })
})
