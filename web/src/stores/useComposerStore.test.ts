import { describe, it, expect, beforeEach, vi } from 'vitest'
import { useComposerStore } from './useComposerStore'

beforeEach(() => {
  vi.restoreAllMocks()
  if (typeof URL.createObjectURL !== 'function') {
    Object.defineProperty(URL, 'createObjectURL', { configurable: true, value: vi.fn() })
  }
  if (typeof URL.revokeObjectURL !== 'function') {
    Object.defineProperty(URL, 'revokeObjectURL', { configurable: true, value: vi.fn() })
  }
  useComposerStore.setState({ composerText: '', attachments: [] })
})

describe('useComposerStore', () => {
  it('starts with empty text', () => {
    expect(useComposerStore.getState().composerText).toBe('')
  })

  it('setComposerText updates the value', () => {
    useComposerStore.getState().setComposerText('hello')
    expect(useComposerStore.getState().composerText).toBe('hello')
  })

  it('overwrites existing text on subsequent setComposerText calls', () => {
    useComposerStore.getState().setComposerText('first')
    useComposerStore.getState().setComposerText('second')
    expect(useComposerStore.getState().composerText).toBe('second')
  })

  it('adds image attachments with preview URLs', () => {
    const createObjectURL = vi.spyOn(URL, 'createObjectURL').mockReturnValue('blob:preview-1')
    const file = new File(['img'], 'a.png', { type: 'image/png' })

    useComposerStore.getState().addAttachmentFiles([file])

    const [attachment] = useComposerStore.getState().attachments
    expect(createObjectURL).toHaveBeenCalledWith(file)
    expect(attachment).toMatchObject({
      file,
      previewUrl: 'blob:preview-1',
      status: 'pending',
    })
  })

  it('revokes preview URL when removing attachments', () => {
    vi.spyOn(URL, 'createObjectURL').mockReturnValue('blob:preview-1')
    const revokeObjectURL = vi.spyOn(URL, 'revokeObjectURL').mockImplementation(() => {})
    useComposerStore.getState().addAttachmentFiles([new File(['img'], 'a.png', { type: 'image/png' })])
    const attachmentId = useComposerStore.getState().attachments[0].id

    useComposerStore.getState().removeAttachment(attachmentId)

    expect(useComposerStore.getState().attachments).toEqual([])
    expect(revokeObjectURL).toHaveBeenCalledWith('blob:preview-1')
  })

  it('can clear sent attachments without revoking object URLs', () => {
    vi.spyOn(URL, 'createObjectURL').mockReturnValue('blob:preview-1')
    const revokeObjectURL = vi.spyOn(URL, 'revokeObjectURL').mockImplementation(() => {})
    useComposerStore.getState().addAttachmentFiles([new File(['img'], 'a.png', { type: 'image/png' })])

    useComposerStore.getState().clearAttachments(false)

    expect(useComposerStore.getState().attachments).toEqual([])
    expect(revokeObjectURL).not.toHaveBeenCalled()
  })

  it('stores upload status and errors per attachment', () => {
    vi.spyOn(URL, 'createObjectURL').mockReturnValue('blob:preview-1')
    useComposerStore.getState().addAttachmentFiles([new File(['img'], 'a.png', { type: 'image/png' })])
    const attachmentId = useComposerStore.getState().attachments[0].id

    useComposerStore.getState().setAttachmentStatus(attachmentId, 'error', 'too large')

    expect(useComposerStore.getState().attachments[0]).toMatchObject({
      status: 'error',
      error: 'too large',
    })
  })
})
