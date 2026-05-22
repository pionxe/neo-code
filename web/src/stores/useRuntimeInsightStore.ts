import { create } from 'zustand'
import {
  type AcceptanceDecidedPayload,
  type BudgetCheckedPayload,
  type BudgetEstimateFailedPayload,
  type CheckpointCreatedPayload,
  type CheckpointDiffResultPayload,
  type CheckpointRestoredPayload,
  type CheckpointUndoRestorePayload,
  type CheckpointWarningPayload,
  type LedgerReconciledPayload,
  type TodoEventPayload,
  type TodoSnapshot,
  type TodoViewItem,
  type VerificationCompletedPayload,
  type VerificationFailedPayload,
} from '@/api/protocol'

export interface TodoHistoryEntry extends TodoViewItem {
  lastSeenAt: number
  firstSeenAt: number
}

interface RuntimeInsightState {
  checkpointDiff: CheckpointDiffResultPayload | null
  checkpointEvents: Array<CheckpointCreatedPayload | CheckpointRestoredPayload | CheckpointUndoRestorePayload>
  checkpointWarning: CheckpointWarningPayload | null
  verificationCompleted: VerificationCompletedPayload | null
  verificationFailed: VerificationFailedPayload | null
  acceptanceDecision: AcceptanceDecidedPayload | null
  todoSnapshot: TodoSnapshot | null
  todoEvents: TodoEventPayload[]
  todoConflict: TodoEventPayload | null
  todoHistory: Record<string, TodoHistoryEntry>
  budgetChecked: BudgetCheckedPayload | null
  budgetEstimateFailed: BudgetEstimateFailedPayload | null
  ledgerReconciled: LedgerReconciledPayload | null
  budgetUsageRatio: number | null

  setCheckpointDiff: (diff: CheckpointDiffResultPayload | null) => void
  addCheckpointEvent: (event: CheckpointCreatedPayload | CheckpointRestoredPayload | CheckpointUndoRestorePayload) => void
  setCheckpointWarning: (warning: CheckpointWarningPayload | null) => void
  completeVerification: (payload: VerificationCompletedPayload) => void
  failVerification: (payload: VerificationFailedPayload) => void
  setAcceptanceDecision: (payload: AcceptanceDecidedPayload | null) => void
  setTodoSnapshot: (snapshot: TodoSnapshot | null) => void
  applyTodoSnapshot: (snapshot: TodoSnapshot | null, options?: { clearConflict?: boolean }) => void
  addTodoEvent: (event: TodoEventPayload) => void
  setTodoConflict: (event: TodoEventPayload | null) => void
  setBudgetChecked: (payload: BudgetCheckedPayload) => void
  setBudgetEstimateFailed: (payload: BudgetEstimateFailedPayload | null) => void
  setLedgerReconciled: (payload: LedgerReconciledPayload | null) => void
  reset: () => void
}

const initialState = {
  checkpointDiff: null as CheckpointDiffResultPayload | null,
  checkpointEvents: [] as Array<CheckpointCreatedPayload | CheckpointRestoredPayload | CheckpointUndoRestorePayload>,
  checkpointWarning: null as CheckpointWarningPayload | null,
  verificationCompleted: null as VerificationCompletedPayload | null,
  verificationFailed: null as VerificationFailedPayload | null,
  acceptanceDecision: null as AcceptanceDecidedPayload | null,
  todoSnapshot: null as TodoSnapshot | null,
  todoEvents: [] as TodoEventPayload[],
  todoConflict: null as TodoEventPayload | null,
  todoHistory: {} as Record<string, TodoHistoryEntry>,
  budgetChecked: null as BudgetCheckedPayload | null,
  budgetEstimateFailed: null as BudgetEstimateFailedPayload | null,
  ledgerReconciled: null as LedgerReconciledPayload | null,
  budgetUsageRatio: null as number | null,
}

function calculateBudgetUsageRatio(payload: BudgetCheckedPayload): number | null {
  if (!payload.prompt_budget || payload.prompt_budget <= 0) return null
  return payload.estimated_input_tokens / payload.prompt_budget
}

export const useRuntimeInsightStore = create<RuntimeInsightState>((set) => ({
  ...initialState,

  setCheckpointDiff: (checkpointDiff) => set({ checkpointDiff }),
  addCheckpointEvent: (event) => set((s) => ({ checkpointEvents: [...s.checkpointEvents, event] })),
  setCheckpointWarning: (checkpointWarning) => set({ checkpointWarning }),
  completeVerification: (verificationCompleted) => set({
    verificationCompleted,
    verificationFailed: null,
  }),
  failVerification: (verificationFailed) => set({
    verificationFailed,
    verificationCompleted: null,
  }),
  setAcceptanceDecision: (acceptanceDecision) => set({ acceptanceDecision }),
  setTodoSnapshot: (todoSnapshot) => set((s) => {
    const items = todoSnapshot?.items ?? []
    if (!todoSnapshot) {
      return { todoSnapshot: null, todoConflict: null }
    }
    if (items.length === 0) {
      return { todoSnapshot, todoConflict: null }
    }
    const now = Date.now()
    const todoHistory = { ...s.todoHistory }
    for (const item of items) {
      const prev = todoHistory[item.id]
      todoHistory[item.id] = {
        ...item,
        lastSeenAt: now,
        firstSeenAt: prev?.firstSeenAt ?? now,
      }
    }
    return { todoSnapshot, todoConflict: null, todoHistory }
  }),
  applyTodoSnapshot: (todoSnapshot, options) => set((s) => {
    const items = todoSnapshot?.items ?? []
    if (!todoSnapshot) {
      return options?.clearConflict ? { todoSnapshot: null, todoConflict: null } : { todoSnapshot: null }
    }
    if (items.length === 0) {
      return options?.clearConflict ? { todoSnapshot, todoConflict: null } : { todoSnapshot }
    }
    const now = Date.now()
    const todoHistory = { ...s.todoHistory }
    for (const item of items) {
      const prev = todoHistory[item.id]
      todoHistory[item.id] = {
        ...item,
        lastSeenAt: now,
        firstSeenAt: prev?.firstSeenAt ?? now,
      }
    }
    return options?.clearConflict
      ? { todoSnapshot, todoConflict: null, todoHistory }
      : { todoSnapshot, todoHistory }
  }),
  addTodoEvent: (event) => set((s) => ({ todoEvents: [...s.todoEvents, event] })),
  setTodoConflict: (todoConflict) => set({ todoConflict }),
  setBudgetChecked: (budgetChecked) => set({
    budgetChecked,
    budgetUsageRatio: calculateBudgetUsageRatio(budgetChecked),
  }),
  setBudgetEstimateFailed: (budgetEstimateFailed) => set({ budgetEstimateFailed }),
  setLedgerReconciled: (ledgerReconciled) => set({ ledgerReconciled }),
  reset: () => set(initialState),
}))
