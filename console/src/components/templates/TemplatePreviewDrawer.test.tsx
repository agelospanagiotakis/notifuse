import '../../__tests__/resizeObserverMock'
import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { App, ConfigProvider } from 'antd'
import TemplatePreviewDrawer from './TemplatePreviewDrawer'
import type { Template, Workspace } from '../../services/api/types'
import type { MessageHistory } from '../../services/api/messages_history'

vi.mock('../../services/api/template', () => ({
  templatesApi: {
    // The compile call is irrelevant here: the Template Data and Metadata tabs are built
    // from props, and the tab bar renders whether or not a preview compiled.
    compile: vi.fn().mockRejectedValue(new Error('no preview in tests'))
  }
}))

const template = {
  id: 'tpl-1',
  name: 'Order Confirmation',
  version: 1,
  category: 'transactional',
  email: {}
} as unknown as Template

const workspace = {
  id: 'ws-1',
  name: 'Test Workspace',
  settings: { timezone: 'UTC' },
  integrations: []
} as unknown as Workspace

const message = (metadata?: Record<string, unknown>): MessageHistory =>
  ({
    id: 'msg-1',
    contact_email: 'bob@example.com',
    template_id: 'tpl-1',
    template_version: 1,
    channel: 'email',
    message_data: { data: {}, ...(metadata === undefined ? {} : { metadata }) },
    sent_at: '2026-08-01T10:00:00Z',
    created_at: '2026-08-01T10:00:00Z',
    updated_at: '2026-08-01T10:00:00Z'
  }) as MessageHistory

const openDrawer = async (messageHistory?: MessageHistory) => {
  render(
    <ConfigProvider>
      <App>
        <TemplatePreviewDrawer record={template} workspace={workspace} messageHistory={messageHistory}>
          <button>open</button>
        </TemplatePreviewDrawer>
      </App>
    </ConfigProvider>
  )

  fireEvent.click(screen.getByText('open'))
  await waitFor(() => expect(screen.getByText('Template Data')).toBeInTheDocument())
}

describe('TemplatePreviewDrawer metadata tab', () => {
  it('adds a Metadata tab when the message carries metadata', async () => {
    await openDrawer(message({ order_id: '9912' }))

    expect(screen.getByText('Metadata')).toBeInTheDocument()
  })

  it('omits the tab when metadata is an empty object', async () => {
    await openDrawer(message({}))

    expect(screen.queryByText('Metadata')).not.toBeInTheDocument()
  })

  it('omits the tab when the message has no metadata', async () => {
    // The broadcast and automation shape.
    await openDrawer(message())

    expect(screen.queryByText('Metadata')).not.toBeInTheDocument()
  })

  it('omits the tab when previewing a template with no message at all', async () => {
    // The drawer is also opened straight from the templates list, with no send behind it.
    await openDrawer()

    expect(screen.queryByText('Metadata')).not.toBeInTheDocument()
  })
})

describe('TemplatePreviewDrawer driven by the templates table', () => {
  const renderControlled = (open: boolean, onOpenChange = vi.fn()) =>
    render(
      <ConfigProvider>
        <App>
          <TemplatePreviewDrawer
            record={template}
            workspace={workspace}
            open={open}
            onOpenChange={onOpenChange}
          />
        </App>
      </ConfigProvider>
    )

  it('opens with no trigger of its own', async () => {
    renderControlled(true)

    // No children were passed, so there is no wrapper to click: the drawer is the entry point.
    await waitFor(() => expect(screen.getByText('Template Data')).toBeInTheDocument())
  })

  it('reports a close request back to the caller', async () => {
    const onOpenChange = vi.fn()
    renderControlled(true, onOpenChange)

    const closeButton = await screen.findByLabelText('Close')
    fireEvent.click(closeButton)

    expect(onOpenChange).toHaveBeenCalledWith(false)
  })

  it('toggles between the readable and the expanded width', async () => {
    renderControlled(true)

    // Width lives in the drawer panel style, so the button's own label is the observable state.
    const expand = await screen.findByRole('button', { name: 'Expand preview' })
    fireEvent.click(expand)
    expect(await screen.findByRole('button', { name: 'Collapse preview' })).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Collapse preview' }))
    expect(await screen.findByRole('button', { name: 'Expand preview' })).toBeInTheDocument()
  })
})
