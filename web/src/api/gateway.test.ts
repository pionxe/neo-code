import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { GatewayAPI } from './gateway'
import { Method } from './protocol'

describe('GatewayAPI', () => {
	const call = vi.fn()
	const ws = { call } as any
	let api: GatewayAPI

	beforeEach(() => {
		call.mockReset()
		call.mockResolvedValue({ type: 'ack', payload: {} })
		api = new GatewayAPI(ws)
	})

	afterEach(() => {
		vi.unstubAllGlobals()
	})

	it('maps authenticate and run methods', async () => {
		await api.authenticate('tok')
		await api.run({ input_text: 'hello' })

		expect(call).toHaveBeenNthCalledWith(1, Method.Authenticate, { token: 'tok' })
		expect(call).toHaveBeenNthCalledWith(2, Method.Run, { input_text: 'hello' })
	})

	it('maps createSession method', async () => {
		await api.createSession()
		await api.createSession('s1')

		expect(call).toHaveBeenNthCalledWith(1, Method.CreateSession, {})
		expect(call).toHaveBeenNthCalledWith(2, Method.CreateSession, { session_id: 's1' })
	})

	it('maps optional session_id in listModels', async () => {
		await api.listModels()
		await api.listModels('s1')

		expect(call).toHaveBeenNthCalledWith(1, Method.ListModels, undefined)
		expect(call).toHaveBeenNthCalledWith(2, Method.ListModels, { session_id: 's1' })
	})

	it('maps optional provider_id in setSessionModel', async () => {
		await api.setSessionModel('s1', 'm1')
		await api.setSessionModel('s1', 'm1', 'p1')

		expect(call).toHaveBeenNthCalledWith(1, Method.SetSessionModel, { session_id: 's1', model_id: 'm1' })
		expect(call).toHaveBeenNthCalledWith(2, Method.SetSessionModel, { session_id: 's1', model_id: 'm1', provider_id: 'p1' })
	})

	it('maps workspace methods and optional remove_data', async () => {
		await api.listWorkspaces()
		await api.createWorkspace('/tmp/a', 'A')
		await api.switchWorkspace('h1')
		await api.renameWorkspace('h1', 'B')
		await api.deleteWorkspace('h1', true)

		expect(call).toHaveBeenNthCalledWith(1, Method.ListWorkspaces)
		expect(call).toHaveBeenNthCalledWith(2, Method.CreateWorkspace, { path: '/tmp/a', name: 'A' })
		expect(call).toHaveBeenNthCalledWith(3, Method.SwitchWorkspace, { workspace_hash: 'h1' })
		expect(call).toHaveBeenNthCalledWith(4, Method.RenameWorkspace, { workspace_hash: 'h1', name: 'B' })
		expect(call).toHaveBeenNthCalledWith(5, Method.DeleteWorkspace, { workspace_hash: 'h1', remove_data: true })
	})

	it('maps permission and user question resolution', async () => {
		await api.resolvePermission({ request_id: 'r1', decision: 'allow_once' })
		await api.approvePlan({ session_id: 's1', plan_id: 'p1', revision: 2 })
		await api.resolveUserQuestion({ request_id: 'q1', status: 'answered', message: 'ok' })

		expect(call).toHaveBeenNthCalledWith(1, Method.ResolvePermission, { request_id: 'r1', decision: 'allow_once' })
		expect(call).toHaveBeenNthCalledWith(2, Method.ApprovePlan, { session_id: 's1', plan_id: 'p1', revision: 2 })
		expect(call).toHaveBeenNthCalledWith(3, Method.UserQuestionAnswer, { request_id: 'q1', status: 'answered', message: 'ok' })
	})

	it('uploads session assets with bearer auth, workspace header, and multipart body', async () => {
		const fetchMock = vi.fn().mockResolvedValue({
			ok: true,
			json: () => Promise.resolve({ session_id: 's1', asset_id: 'asset-1', mime_type: 'image/png', size: 3 }),
		})
		vi.stubGlobal('fetch', fetchMock)
		api = new GatewayAPI(ws, 'http://localhost:1455/', ' token-1 ')

		const file = new File(['abc'], 'a.png', { type: 'image/png' })
		const result = await api.uploadSessionAsset('s1', file, 'workspace-b')

		expect(result.asset_id).toBe('asset-1')
		expect(fetchMock).toHaveBeenCalledWith('http://localhost:1455/api/session-assets', expect.objectContaining({
			method: 'POST',
			headers: { Authorization: 'Bearer token-1', 'X-NeoCode-Workspace-Hash': 'workspace-b' },
		}))
		const init = fetchMock.mock.calls[0][1] as RequestInit
		expect(init.body).toBeInstanceOf(FormData)
		expect((init.body as FormData).get('session_id')).toBe('s1')
		expect((init.body as FormData).get('file')).toBe(file)
	})

	it('fetches session asset blobs with bearer auth and workspace header', async () => {
		const blob = new Blob(['img'], { type: 'image/png' })
		const fetchMock = vi.fn().mockResolvedValue({
			ok: true,
			blob: () => Promise.resolve(blob),
		})
		vi.stubGlobal('fetch', fetchMock)
		api = new GatewayAPI(ws, '/gateway', 'token-1')

		await expect(api.fetchSessionAsset('s 1', 'asset/1', 'workspace-b')).resolves.toBe(blob)
		expect(fetchMock).toHaveBeenCalledWith('/gateway/api/session-assets/s%201/asset%2F1', {
			headers: { Authorization: 'Bearer token-1', 'X-NeoCode-Workspace-Hash': 'workspace-b' },
		})
	})

	it('deletes session assets with bearer auth and workspace header', async () => {
		const fetchMock = vi.fn().mockResolvedValue({ ok: true })
		vi.stubGlobal('fetch', fetchMock)
		api = new GatewayAPI(ws, '/gateway', 'token-1')

		await api.deleteSessionAsset('s 1', 'asset/1', 'workspace-b')

		expect(fetchMock).toHaveBeenCalledWith('/gateway/api/session-assets/s%201/asset%2F1', {
			method: 'DELETE',
			headers: { Authorization: 'Bearer token-1', 'X-NeoCode-Workspace-Hash': 'workspace-b' },
		})
	})

	it('uses switched workspace as session asset HTTP fallback', async () => {
		call.mockResolvedValueOnce({ type: 'ack', payload: { workspace_hash: 'workspace-c' } })
		const fetchMock = vi.fn().mockResolvedValue({
			ok: true,
			blob: () => Promise.resolve(new Blob(['img'])),
		})
		vi.stubGlobal('fetch', fetchMock)
		api = new GatewayAPI(ws, '', 'token-1')

		await api.switchWorkspace('workspace-c')
		await api.fetchSessionAsset('s1', 'asset-1')
		await api.deleteSessionAsset('s1', 'asset-1')

		expect(fetchMock).toHaveBeenNthCalledWith(1, '/api/session-assets/s1/asset-1', {
			headers: { Authorization: 'Bearer token-1', 'X-NeoCode-Workspace-Hash': 'workspace-c' },
		})
		expect(fetchMock).toHaveBeenNthCalledWith(2, '/api/session-assets/s1/asset-1', {
			method: 'DELETE',
			headers: { Authorization: 'Bearer token-1', 'X-NeoCode-Workspace-Hash': 'workspace-c' },
		})
	})

	it('surfaces session asset HTTP errors', async () => {
		vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
			ok: false,
			status: 415,
			json: () => Promise.resolve({ error: 'unsupported image type' }),
		}))
		await expect(api.uploadSessionAsset('s1', new File(['x'], 'x.txt'))).rejects.toThrow('unsupported image type')
	})
})

