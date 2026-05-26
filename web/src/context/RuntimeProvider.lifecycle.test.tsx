import { act, render, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { RuntimeProvider, useRuntime } from './RuntimeProvider'
import { useChatStore } from '@/stores/useChatStore'
import { useSessionStore } from '@/stores/useSessionStore'
import { useWorkspaceStore } from '@/stores/useWorkspaceStore'

const clients: any[] = []

vi.mock('@/api/wsClient', () => ({
	createWSClient: vi.fn(() => {
		let onState: ((s: any) => void) | null = null
		let onEvent: ((f: any) => void) | null = null
		let onReconnect: (() => void) | null = null
		const client = {
			connect: vi.fn(() => onState?.('connected')),
			disconnect: vi.fn(() => onState?.('disconnected')),
			reconnect: vi.fn(),
			call: vi.fn(async (method: string) => {
				if (method === 'gateway.authenticate') return { payload: {} }
				if (method === 'gateway.listWorkspaces') return { payload: { workspaces: [] } }
				if (method === 'gateway.ping') return { payload: {} }
				return { payload: {} }
			}),
			onEvent: vi.fn((h: any) => {
				onEvent = h
				return () => {
					if (onEvent === h) onEvent = null
				}
			}),
			onStateChange: vi.fn((h: any) => {
				onState = h
				return () => {
					if (onState === h) onState = null
				}
			}),
			onReconnect: vi.fn((h: any) => {
				onReconnect = h
				return () => {
					if (onReconnect === h) onReconnect = null
				}
			}),
			_emitState: (s: any) => onState?.(s),
			_emitReconnect: () => onReconnect?.(),
		}
		clients.push(client)
		return client
	}),
}))

function RuntimeProbe({ onReady }: { onReady: (value: ReturnType<typeof useRuntime>) => void }) {
	const runtime = useRuntime()
	onReady(runtime)
	return null
}

describe('RuntimeProvider lifecycle', () => {
	beforeEach(() => {
		clients.length = 0
		sessionStorage.clear()
		Object.defineProperty(window.navigator, 'userAgent', {
			value: 'Mozilla/5.0 Chrome/120 Safari/537.36',
			configurable: true,
		})
		Object.defineProperty(window, 'electronAPI', {
			value: undefined,
			configurable: true,
			writable: true,
		})

		useSessionStore.setState({
			fetchSessions: vi.fn().mockResolvedValue(undefined),
			initializeActiveSession: vi.fn().mockResolvedValue(undefined),
			setProjects: vi.fn(),
			setCurrentSessionId: vi.fn(),
			setCurrentProjectId: vi.fn(),
			currentSessionId: '',
		} as any)
		useWorkspaceStore.setState({
			fetchWorkspaces: vi.fn().mockResolvedValue(undefined),
			setWorkspaces: vi.fn(),
			setCurrentWorkspaceHash: vi.fn(),
			workspaces: [],
		} as any)
		useChatStore.setState({
			clearMessages: vi.fn(),
			clearPendingUserQuestion: vi.fn(),
			resetGeneratingState: vi.fn(),
		} as any)
	})

	it('connects from stored browser config and exposes connected runtime', async () => {
		sessionStorage.setItem(
			'neocode.browserRuntimeConfig',
			JSON.stringify({ mode: 'browser', gatewayBaseURL: 'http://127.0.0.1:8080', token: 'tok' }),
		)
		let runtimeSnapshot: any = null
		render(
			<RuntimeProvider>
				<RuntimeProbe onReady={(rt) => { runtimeSnapshot = rt }} />
			</RuntimeProvider>,
		)

		await waitFor(() => {
			expect(runtimeSnapshot?.status).toBe('connected')
			expect(runtimeSnapshot?.gatewayAPI).toBeTruthy()
		})
		expect(clients).toHaveLength(1)
		expect(clients[0].connect).toHaveBeenCalled()
	})

	it('retry reconnects with existing config', async () => {
		sessionStorage.setItem(
			'neocode.browserRuntimeConfig',
			JSON.stringify({ mode: 'browser', gatewayBaseURL: 'http://127.0.0.1:8080', token: 'tok' }),
		)
		let runtimeSnapshot: any = null
		render(
			<RuntimeProvider>
				<RuntimeProbe onReady={(rt) => { runtimeSnapshot = rt }} />
			</RuntimeProvider>,
		)
		await waitFor(() => expect(runtimeSnapshot?.status).toBe('connected'))

		await act(async () => {
			await runtimeSnapshot.retry()
		})
		expect(clients.length).toBeGreaterThanOrEqual(2)
	})

	it('resetBrowserConfig clears store-facing runtime state', async () => {
		sessionStorage.setItem(
			'neocode.browserRuntimeConfig',
			JSON.stringify({ mode: 'browser', gatewayBaseURL: 'http://127.0.0.1:8080', token: 'tok' }),
		)
		let runtimeSnapshot: any = null
		const chatClear = useChatStore.getState().clearMessages as any
		render(
			<RuntimeProvider>
				<RuntimeProbe onReady={(rt) => { runtimeSnapshot = rt }} />
			</RuntimeProvider>,
		)
		await waitFor(() => expect(runtimeSnapshot?.status).toBe('connected'))

		act(() => {
			runtimeSnapshot.resetBrowserConfig()
		})

		expect(sessionStorage.getItem('neocode.browserRuntimeConfig')).toBeNull()
		expect(chatClear).toHaveBeenCalled()
		expect(runtimeSnapshot.status).toBe('needs_config')
	})

	it('pickWorkspaceDirectory is a no-op in browser mode', async () => {
		sessionStorage.setItem(
			'neocode.browserRuntimeConfig',
			JSON.stringify({ mode: 'browser', gatewayBaseURL: 'http://127.0.0.1:8080', token: 'tok' }),
		)
		let runtimeSnapshot: any = null
		render(
			<RuntimeProvider>
				<RuntimeProbe onReady={(rt) => { runtimeSnapshot = rt }} />
			</RuntimeProvider>,
		)
		await waitFor(() => expect(runtimeSnapshot?.status).toBe('connected'))

		let selected: string | null = 'unexpected'
		await act(async () => {
			selected = await runtimeSnapshot.pickWorkspaceDirectory()
		})

		expect(selected).toBeNull()
		expect(runtimeSnapshot.workdir).toBe('')
	})

	it('pickWorkspaceDirectory returns the selected Electron directory without restarting the Gateway', async () => {
		const pickDirectory = vi.fn().mockResolvedValue({
			canceled: false,
			filePaths: ['D:\\projects\\neo-code'],
		})
		const selectWorkdir = vi.fn().mockResolvedValue({ canceled: false, workdir: 'D:\\other' })
		Object.defineProperty(window, 'electronAPI', {
			value: {
				getAddress: vi.fn().mockResolvedValue('127.0.0.1:8080'),
				getToken: vi.fn().mockResolvedValue('tok'),
				getWorkdir: vi.fn().mockResolvedValue('D:\\initial'),
				pickDirectory,
				selectWorkdir,
			},
			configurable: true,
			writable: true,
		})
		let runtimeSnapshot: any = null
		render(
			<RuntimeProvider>
				<RuntimeProbe onReady={(rt) => { runtimeSnapshot = rt }} />
			</RuntimeProvider>,
		)
		await waitFor(() => expect(runtimeSnapshot?.status).toBe('connected'))
		await waitFor(() => expect(runtimeSnapshot?.workdir).toBe('D:\\initial'))

		let selected: string | null = ''
		await act(async () => {
			selected = await runtimeSnapshot.pickWorkspaceDirectory()
		})

		expect(selected).toBe('D:\\projects\\neo-code')
		expect(pickDirectory).toHaveBeenCalledTimes(1)
		expect(selectWorkdir).not.toHaveBeenCalled()
		expect(runtimeSnapshot.workdir).toBe('D:\\initial')
	})

	it('pickWorkspaceDirectory returns null when Electron directory selection is canceled', async () => {
		const pickDirectory = vi.fn().mockResolvedValue({
			canceled: true,
			filePaths: ['D:\\projects\\ignored'],
		})
		Object.defineProperty(window, 'electronAPI', {
			value: {
				getAddress: vi.fn().mockResolvedValue('127.0.0.1:8080'),
				getToken: vi.fn().mockResolvedValue('tok'),
				getWorkdir: vi.fn().mockResolvedValue('D:\\initial'),
				pickDirectory,
				selectWorkdir: vi.fn(),
			},
			configurable: true,
			writable: true,
		})
		let runtimeSnapshot: any = null
		render(
			<RuntimeProvider>
				<RuntimeProbe onReady={(rt) => { runtimeSnapshot = rt }} />
			</RuntimeProvider>,
		)
		await waitFor(() => expect(runtimeSnapshot?.status).toBe('connected'))
		await waitFor(() => expect(runtimeSnapshot?.workdir).toBe('D:\\initial'))

		let selected: string | null = ''
		await act(async () => {
			selected = await runtimeSnapshot.pickWorkspaceDirectory()
		})

		expect(selected).toBeNull()
		expect(pickDirectory).toHaveBeenCalledTimes(1)
		expect(runtimeSnapshot.workdir).toBe('D:\\initial')
	})

	it('restores workspace context before rebinding session on reconnect', async () => {
		sessionStorage.setItem(
			'neocode.browserRuntimeConfig',
			JSON.stringify({ mode: 'browser', gatewayBaseURL: 'http://127.0.0.1:8080', token: 'tok' }),
		)
		useWorkspaceStore.setState({
			fetchWorkspaces: vi.fn().mockResolvedValue(undefined),
			workspaces: [{ hash: 'w2', path: '/workspace-two', name: 'Two', createdAt: '', updatedAt: '' }],
			currentWorkspaceHash: 'w2',
		} as any)
		useSessionStore.setState({
			...useSessionStore.getState(),
			currentSessionId: 'session-2',
			fetchSessions: vi.fn(async (gatewayAPI: any) => {
				await gatewayAPI.listSessions()
			}),
			projects: [{
				id: 'group_today',
				name: 'Today',
				sessions: [{ id: 'session-2', title: 'Two', time: new Date(0).toISOString() }],
			}],
		} as any)

		let runtimeSnapshot: any = null
		render(
			<RuntimeProvider>
				<RuntimeProbe onReady={(rt) => { runtimeSnapshot = rt }} />
			</RuntimeProvider>,
		)
		await waitFor(() => expect(runtimeSnapshot?.status).toBe('connected'))
		const client = clients[0]
		client.call.mockClear()

		await act(async () => {
			await client._emitReconnect()
		})

		const methods = client.call.mock.calls.map((call: any[]) => call[0])
		const switchIndex = methods.indexOf('gateway.switchWorkspace')
		const fetchIndex = methods.indexOf('gateway.listSessions')
		const bindIndex = methods.indexOf('gateway.bindStream')
		expect(switchIndex).toBeGreaterThanOrEqual(0)
		expect(fetchIndex).toBeGreaterThanOrEqual(0)
		expect(bindIndex).toBeGreaterThanOrEqual(0)
		expect(switchIndex).toBeLessThan(fetchIndex)
		expect(fetchIndex).toBeLessThan(bindIndex)
		expect(client.call).toHaveBeenCalledWith('gateway.switchWorkspace', { workspace_hash: 'w2' })
		expect(client.call).toHaveBeenCalledWith('gateway.bindStream', { session_id: 'session-2', channel: 'all' })
	})

	it('recovers reconnect when rebinding a stale session fails', async () => {
		sessionStorage.setItem(
			'neocode.browserRuntimeConfig',
			JSON.stringify({ mode: 'browser', gatewayBaseURL: 'http://127.0.0.1:8080', token: 'tok' }),
		)
		useWorkspaceStore.setState({
			fetchWorkspaces: vi.fn().mockResolvedValue(undefined),
			workspaces: [{ hash: 'w2', path: '/workspace-two', name: 'Two', createdAt: '', updatedAt: '' }],
			currentWorkspaceHash: 'w2',
		} as any)
		useSessionStore.setState({
			...useSessionStore.getState(),
			currentSessionId: 'session-stale',
			fetchSessions: vi.fn().mockResolvedValue(undefined),
			projects: [{
				id: 'group_today',
				name: 'Today',
				sessions: [{ id: 'session-stale', title: 'Stale', time: new Date(0).toISOString() }],
			}],
		} as any)
		const warnSpy = vi.spyOn(console, 'warn').mockImplementation(() => {})

		let runtimeSnapshot: any = null
		render(
			<RuntimeProvider>
				<RuntimeProbe onReady={(rt) => { runtimeSnapshot = rt }} />
			</RuntimeProvider>,
		)
		await waitFor(() => expect(runtimeSnapshot?.status).toBe('connected'))
		const client = clients[0]
		client.call.mockImplementation(async (method: string, params?: any) => {
			if (method === 'gateway.bindStream' && params?.session_id === 'session-stale') {
				throw new Error('session not found')
			}
			if (method === 'gateway.authenticate') return { payload: {} }
			if (method === 'gateway.listWorkspaces') return { payload: { workspaces: [] } }
			if (method === 'gateway.ping') return { payload: {} }
			return { payload: {} }
		})

		await act(async () => {
			await client._emitReconnect()
		})

		await waitFor(() => expect(runtimeSnapshot?.status).toBe('connected'))
		expect(runtimeSnapshot.error).toBe('')
		expect(useSessionStore.getState().setCurrentSessionId).toHaveBeenCalledWith('')
		warnSpy.mockRestore()
	})
})

