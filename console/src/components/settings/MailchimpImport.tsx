import { useCallback, useEffect, useState } from 'react'
import { Alert, Button, Card, Form, Input, Space, Spin, Table, Tag, Typography, message } from 'antd'
import { api } from '../../services/api/client'

const { Text, Paragraph } = Typography

interface ImportTool {
  id: string
  title: string
  why: string
  done: boolean
  done_count: number
  total: number
  unit: string
  command: string
  note?: string
}

interface Status {
  tools: ImportTool[]
  key_set: boolean
  workspace: string
  checked_at: string
}

interface Inventory {
  campaigns: number
  audiences: number
  subscribers: number
}

/**
 * One tool per import type, each with a measured status and the exact command for
 * the part that is not done.
 *
 * The screen deliberately does not claim to run the import: it takes hours, it must
 * survive deploys and resume mid-way, and it downloads gigabytes — that is a worker's
 * job, and the worker exists. What was missing was never the work, it was knowing
 * where you are.
 */
export function MailchimpImport({ workspace }: { workspace: { id: string; name: string } }) {
  const [form] = Form.useForm()
  const [status, setStatus] = useState<Status | null>(null)
  const [inventory, setInventory] = useState<Inventory | null>(null)
  const [loading, setLoading] = useState(true)
  const [savingKey, setSavingKey] = useState(false)
  const [checking, setChecking] = useState(false)

  const load = useCallback(async () => {
    setLoading(true)
    try {
      setStatus(await api.get<Status>(`/api/import.status?workspace_id=${workspace.id}`))
    } catch (error) {
      message.error((error as Error).message || 'Could not load import status')
    } finally {
      setLoading(false)
    }
  }, [workspace.id])

  useEffect(() => {
    void load()
  }, [load])

  const saveKey = async () => {
    const values = await form.validateFields()
    setSavingKey(true)
    try {
      await api.post('/api/import.key', { workspace_id: workspace.id, api_key: values.api_key })
      message.success(values.api_key ? 'Mailchimp key saved and verified' : 'Key cleared')
      form.setFieldValue('api_key', '')
      await load()
    } catch (error) {
      message.error((error as Error).message || 'That key was rejected')
    } finally {
      setSavingKey(false)
    }
  }

  const loadInventory = async () => {
    setChecking(true)
    try {
      setInventory(await api.get<Inventory>(`/api/import.inventory?workspace_id=${workspace.id}`))
    } catch (error) {
      message.error((error as Error).message || 'Could not reach Mailchimp')
    } finally {
      setChecking(false)
    }
  }

  const columns = [
    {
      title: '',
      dataIndex: 'done',
      width: 70,
      render: (done: boolean) => (done ? <Tag color="green">done</Tag> : <Tag>to do</Tag>)
    },
    {
      title: 'Import',
      render: (_: unknown, row: ImportTool) => (
        <Space direction="vertical" size={2} style={{ maxWidth: 620 }}>
          <Text strong>{row.title}</Text>
          <Text type="secondary" style={{ fontSize: 13 }}>
            {row.why}
          </Text>
          {row.note && (
            <Text type="warning" style={{ fontSize: 12 }}>
              {row.note}
            </Text>
          )}
        </Space>
      )
    },
    {
      title: 'Progress',
      width: 150,
      render: (_: unknown, row: ImportTool) =>
        row.total > 0 ? (
          <Text>
            {row.done_count} / {row.total} {row.unit}
          </Text>
        ) : (
          <Text type="secondary">
            {row.done_count} {row.unit}
          </Text>
        )
    },
    {
      title: 'Command',
      width: 420,
      render: (_: unknown, row: ImportTool) => (
        <Space direction="vertical" size={4} style={{ width: '100%' }}>
          <Text code style={{ display: 'block', whiteSpace: 'pre-wrap', wordBreak: 'break-all', fontSize: 11 }}>
            {row.command}
          </Text>
          <Button size="small" onClick={() => void navigator.clipboard?.writeText(row.command)}>
            Copy
          </Button>
        </Space>
      )
    }
  ]

  return (
    <Space direction="vertical" size="large" style={{ width: '100%' }}>
      <Card title="Mailchimp API key" loading={loading}>
        <Paragraph type="secondary">
          Stored encrypted with the deployment secret, and verified against Mailchimp before it is
          saved. It is only used to count what is there and to keep the import honest about how
          much is left.
        </Paragraph>
        <Space align="end">
          <Form form={form} layout="vertical" style={{ width: 420 }}>
            <Form.Item
              name="api_key"
              label={status?.key_set ? 'Key stored — type a new one to replace it' : 'API key'}
              extra="Mailchimp → Account → Extras → API keys"
            >
              <Input.Password placeholder="xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx-us8" autoComplete="off" />
            </Form.Item>
          </Form>
          <Button type="primary" loading={savingKey} onClick={() => void saveKey()} style={{ marginBottom: 24 }}>
            Save key
          </Button>
          <Button
            loading={checking}
            disabled={!status?.key_set}
            onClick={() => void loadInventory()}
            style={{ marginBottom: 24 }}
          >
            Check what is there
          </Button>
        </Space>
        {inventory && (
          <Alert
            type="info"
            showIcon
            message="Mailchimp inventory"
            description={`${inventory.campaigns} sent campaigns · ${inventory.audiences} audiences · ${inventory.subscribers} subscribers`}
          />
        )}
      </Card>

      <Card
        title={`Import tools — ${status?.workspace ?? workspace.name}`}
        extra={status ? <Text type="secondary" style={{ fontSize: 12 }}>measured {new Date(status.checked_at).toLocaleString()}</Text> : null}
      >
        {loading ? (
          <Spin />
        ) : (
          <Table
            rowKey="id"
            size="small"
            pagination={false}
            dataSource={status?.tools ?? []}
            columns={columns}
            tableLayout="fixed"
            scroll={{ x: 1080 }}
            expandable={{ defaultExpandAllRows: true, showExpandColumn: false }}
          />
        )}
      </Card>
    </Space>
  )
}
