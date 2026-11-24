import { useCallback, useEffect, useState } from 'react'
import {
  Alert,
  Button,
  Card,
  Checkbox,
  Form,
  Input,
  Popconfirm,
  Space,
  Switch,
  Table,
  Tag,
  Typography,
  message
} from 'antd'
import { api } from '../../services/api/client'

const { Text, Paragraph, Link } = Typography

interface ArchiveEntry {
  slug: string
  title: string
  subject: string
  sent_at: string
  template_id: string
  preview_url: string
}

interface ArchiveAudience {
  list_id: string
  slug: string
  name: string
  listed: boolean
  notifuse_list_id?: string
  url?: string
  campaigns: number
}

interface ArchiveState {
  settings: {
    enabled: boolean
    show_audience_nav: boolean
    title: string
    subscribe_url: string
    subscribe_list_ids: string[]
    basic_auth_user: string
  }
  gate_set: boolean
  entries: ArchiveEntry[]
  audiences: ArchiveAudience[]
  public_url: string
}

/**
 * The campaign archive, as an operator screen.
 *
 * Everything here used to be SQL on the server, which is the wrong shape for a feature
 * meant to be handed to someone who does not read SQL. The password is never sent to
 * the browser: the screen shows whether a gate is set and replaces it only when a new
 * one is typed.
 */
export function ArchiveSettings({ workspace }: { workspace: { id: string; name: string } }) {
  const [form] = Form.useForm()
  const [state, setState] = useState<ArchiveState | null>(null)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [gateSet, setGateSet] = useState(false)
  const [audiences, setAudiences] = useState<ArchiveAudience[]>([])

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const data = await api.get<ArchiveState>(`/api/archive.get?workspace_id=${workspace.id}`)
      setState(data)
      setGateSet(data.gate_set)
      setAudiences(data.audiences ?? [])
      form.setFieldsValue({
        enabled: data.settings.enabled,
        show_audience_nav: data.settings.show_audience_nav ?? false,
        title: data.settings.title,
        subscribe_url: data.settings.subscribe_url,
        subscribe_list_ids: (data.settings.subscribe_list_ids || []).join(', '),
        basic_auth_user: data.settings.basic_auth_user,
        basic_auth_password: ''
      })
    } catch (error) {
      message.error((error as Error).message || 'Could not load the archive')
    } finally {
      setLoading(false)
    }
  }, [workspace.id, form])

  useEffect(() => {
    void load()
  }, [load])

  const save = async () => {
    const values = await form.validateFields()
    setSaving(true)
    try {
      await api.post('/api/archive.update', {
        workspace_id: workspace.id,
        settings: {
          enabled: values.enabled,
          show_audience_nav: Boolean(values.show_audience_nav),
          title: values.title || '',
          subscribe_url: values.subscribe_url || '',
          subscribe_list_ids: String(values.subscribe_list_ids || '')
            .split(',')
            .map((id: string) => id.trim())
            .filter(Boolean),
          basic_auth_user: values.basic_auth_user || '',
          basic_auth_password: values.basic_auth_password || '',
          clear_gate: !values.basic_auth_user
        }
      })
      message.success('Archive settings saved')
      await load()
    } catch (error) {
      message.error((error as Error).message || 'Could not save')
    } finally {
      setSaving(false)
    }
  }

  const remove = async (slug: string) => {
    try {
      await api.post('/api/archive.remove', { workspace_id: workspace.id, slug })
      message.success('Removed from the archive. The template itself is untouched.')
      await load()
    } catch (error) {
      message.error((error as Error).message || 'Could not remove')
    }
  }

  const setListed = async (listID: string, listed: boolean) => {
    try {
      await api.post('/api/archive.setAudienceListed', {
        workspace_id: workspace.id,
        list_id: listID,
        listed
      })
      setAudiences((prev) => prev.map((a) => (a.list_id === listID ? { ...a, listed } : a)))
    } catch (error) {
      message.error((error as Error).message || 'Could not change the audience listing')
    }
  }

  const columns = [
    {
      title: 'Sent',
      dataIndex: 'sent_at',
      width: 120,
      sorter: (a: ArchiveEntry, b: ArchiveEntry) => a.sent_at.localeCompare(b.sent_at)
    },
    {
      title: 'Campaign',
      dataIndex: 'title',
      render: (title: string, row: ArchiveEntry) => (
        <Space direction="vertical" size={0}>
          <Text strong>{title}</Text>
          <Text type="secondary" style={{ fontSize: 12 }}>
            {row.template_id}
          </Text>
        </Space>
      )
    },
    {
      title: '',
      width: 210,
      render: (_: unknown, row: ArchiveEntry) => (
        <Space>
          <Link href={row.preview_url} target="_blank" rel="noreferrer">
            Preview
          </Link>
          <Popconfirm
            title="Remove from the archive?"
            description="The template stays. The page stops being public."
            okText="Remove"
            onConfirm={() => void remove(row.slug)}
          >
            <Button type="link" danger size="small">
              Remove
            </Button>
          </Popconfirm>
        </Space>
      )
    }
  ]

  return (
    <Space direction="vertical" size="large" style={{ width: '100%' }}>
      <Card loading={loading} title="Campaign archive">
        <Paragraph type="secondary">
          A public page per sent campaign — the Mailchimp campaign archive, on this platform. Off by
          default; nothing is served until it is enabled.
        </Paragraph>

        <Form form={form} layout="vertical" style={{ maxWidth: 560 }}>
          <Form.Item name="enabled" label="Enabled" valuePropName="checked">
            <Switch />
          </Form.Item>
          <Form.Item
            name="show_audience_nav"
            label="Show the audience switch on the public pages"
            valuePropName="checked"
            extra="Off by default: which list a campaign went to is running information, not something a reader needs. The per-audience pages stay available — they are listed below."
          >
            <Switch />
          </Form.Item>
          <Form.Item
            name="title"
            label="Page title"
            extra="The toolbar shows the part before the · separator."
          >
            <Input placeholder={`${workspace.name} · Newsletter archive`} />
          </Form.Item>
          <Form.Item
            name="subscribe_list_ids"
            label="Subscribe form joins these lists"
            extra="Comma-separated public list ids. Empty means Subscribe links out to the URL below instead."
          >
            <Input placeholder="koisnewsletter" />
          </Form.Item>
          <Form.Item name="subscribe_url" label="External signup URL (fallback)">
            <Input placeholder="https://www.example.gr/newsletter" />
          </Form.Item>
          <Form.Item
            name="basic_auth_user"
            label="Basic auth user"
            extra="Leave empty for a public archive. Setting a user hides Translate until it is removed, because a translate proxy cannot reach a gated page."
          >
            <Input placeholder="public" autoComplete="off" />
          </Form.Item>
          <Form.Item
            name="basic_auth_password"
            label="Basic auth password"
            extra={gateSet ? 'A password is set. Leave empty to keep it.' : 'No password set.'}
          >
            <Input.Password autoComplete="new-password" placeholder={gateSet ? '••••••••' : 'none'} />
          </Form.Item>
          <Space>
            <Button type="primary" onClick={() => void save()} loading={saving}>
              Save
            </Button>
            {state && (
              <Link href={state.public_url} target="_blank" rel="noreferrer">
                Open the archive
              </Link>
            )}
          </Space>
        </Form>
      </Card>

      <Card
        title={`Audiences${state ? ` (${audiences.length})` : ''}`}
        extra={
          <Text type="secondary" style={{ fontSize: 12 }}>
            Tick to publish a list&apos;s archive. Unticked it answers 404, not just a hidden link.
          </Text>
        }
      >
        <Table<ArchiveAudience>
          size="small"
          rowKey="list_id"
          pagination={false}
          dataSource={audiences}
          locale={{ emptyText: 'No campaigns name an audience yet.' }}
          columns={[
            {
              title: 'Audience',
              dataIndex: 'name',
              render: (name: string, row: ArchiveAudience) => (
                <Space direction="vertical" size={0}>
                  <Text strong>{name || row.list_id}</Text>
                  <Text type="secondary" style={{ fontSize: 12 }}>
                    {row.slug || row.list_id}
                  </Text>
                </Space>
              )
            },
            {
              title: 'Listed',
              dataIndex: 'listed',
              width: 110,
              render: (listed: boolean, row: ArchiveAudience) => (
                <Space size={6}>
                  <Checkbox checked={listed} onChange={(e) => void setListed(row.list_id, e.target.checked)} />
                  <Text type={listed ? undefined : 'secondary'}>{listed ? 'Public' : 'Hidden'}</Text>
                </Space>
              )
            },
            { title: 'Campaigns', dataIndex: 'campaigns', width: 120, align: 'right' as const },
            {
              title: 'Archive',
              width: 100,
              render: (_: unknown, row: ArchiveAudience) =>
                row.url ? (
                  <Link href={row.url} target="_blank" rel="noreferrer">
                    Open
                  </Link>
                ) : null
            }
          ]}
        />
      </Card>

      <Card
        title={`Archived campaigns${state ? ` (${state.entries.length})` : ''}`}
        extra={state && state.gate_set ? <Tag color="orange">behind basic auth</Tag> : null}
      >
        {state && state.entries.length === 0 && (
          <Alert
            type="info"
            showIcon
            message="Nothing archived yet"
            description="Entries are created by the import or by publishing a template from the import screen."
          />
        )}
        <Table
          rowKey="slug"
          size="small"
          loading={loading}
          dataSource={state?.entries ?? []}
          columns={columns}
          pagination={{ pageSize: 20, hideOnSinglePage: true }}
        />
      </Card>
    </Space>
  )
}
