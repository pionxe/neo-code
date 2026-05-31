import { create } from 'zustand'

export const acceptedImageMimeTypes = ['image/png', 'image/jpeg', 'image/webp'] as const
export const maxComposerAttachmentBytes = 20 * 1024 * 1024

export interface ComposerAttachment {
  id: string
  file: File
  previewUrl: string
  status: 'pending' | 'uploading' | 'uploaded' | 'error'
  error?: string
}

interface ComposerState {
  composerText: string
  attachments: ComposerAttachment[]
  setComposerText: (text: string) => void
  addAttachmentFiles: (files: File[]) => void
  removeAttachment: (id: string) => void
  clearAttachments: (revoke?: boolean) => void
  setAttachmentStatus: (id: string, status: ComposerAttachment['status'], error?: string) => void
}

export const useComposerStore = create<ComposerState>((set) => ({
  composerText: '',
  attachments: [],
  setComposerText: (composerText) => set({ composerText }),
  addAttachmentFiles: (files) => set((state) => ({
    attachments: [
      ...state.attachments,
      ...files.map(createComposerAttachment),
    ],
  })),
  removeAttachment: (id) => set((state) => {
    const target = state.attachments.find((attachment) => attachment.id === id)
    revokePreviewURL(target?.previewUrl)
    return { attachments: state.attachments.filter((attachment) => attachment.id !== id) }
  }),
  clearAttachments: (revoke = true) => set((state) => {
    if (revoke) state.attachments.forEach((attachment) => revokePreviewURL(attachment.previewUrl))
    return { attachments: [] }
  }),
  setAttachmentStatus: (id, status, error) => set((state) => ({
    attachments: state.attachments.map((attachment) => (
      attachment.id === id ? { ...attachment, status, error } : attachment
    )),
  })),
}))

function createComposerAttachment(file: File): ComposerAttachment {
  return {
    id: `att_${Date.now()}_${Math.random().toString(36).slice(2)}`,
    file,
    previewUrl: createPreviewURL(file),
    status: 'pending',
  }
}

function createPreviewURL(file: File) {
  if (typeof URL !== 'undefined' && typeof URL.createObjectURL === 'function') {
    return URL.createObjectURL(file)
  }
  return ''
}

function revokePreviewURL(url?: string) {
  if (!url || typeof URL === 'undefined' || typeof URL.revokeObjectURL !== 'function') return
  URL.revokeObjectURL(url)
}
