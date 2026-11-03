import { Alert, Descriptions, Space, Tag, Typography } from 'antd'

const { Text } = Typography

/**
 * Figures reported by the platform that actually sent this campaign.
 *
 * The console's own statistics are computed from events this platform processed. A
 * campaign imported from Mailchimp was sent before this platform existed, so its
 * native rates are empty — not zero, absent. Rather than write Mailchimp's numbers
 * into our own tables (where nobody could later tell measured from copied), they are
 * shown here, labelled, in their own block.
 *
 * Nothing here is editable: nobody should be able to change a number somebody else
 * measured.
 */
export interface ImportedStats {
  source?: string
  sent?: number
  unique_opens?: number
  unique_clicks?: number
  unsubscribed?: number
  hard_bounces?: number
  soft_bounces?: number
  fetched_at?: string
}

export function ImportedMailchimpStats({ stats }: { stats?: ImportedStats | null }) {
  if (!stats || stats.source !== 'mailchimp' || !stats.sent) {
    return null
  }

  const rate = (value?: number) =>
    value && stats.sent ? `${((value / stats.sent) * 100).toFixed(1)}%` : '—'

  return (
    <Alert
      type="info"
      showIcon
      style={{ marginTop: 16 }}
      message={
        <Space size="small">
          <Text strong>Reported by Mailchimp</Text>
          <Tag>imported</Tag>
        </Space>
      }
      description={
        <>
          <Text type="secondary">
            This campaign was sent before this platform existed. The figures below were measured by
            Mailchimp and are shown as reported — the statistics above count only events this
            platform processed, so they do not include these.
          </Text>
          <Descriptions size="small" column={3} style={{ marginTop: 12 }}>
            <Descriptions.Item label="Sent">{stats.sent.toLocaleString()}</Descriptions.Item>
            <Descriptions.Item label="Opened">
              {(stats.unique_opens ?? 0).toLocaleString()} ({rate(stats.unique_opens)})
            </Descriptions.Item>
            <Descriptions.Item label="Clicked">
              {(stats.unique_clicks ?? 0).toLocaleString()} ({rate(stats.unique_clicks)})
            </Descriptions.Item>
            <Descriptions.Item label="Hard bounces">{stats.hard_bounces ?? 0}</Descriptions.Item>
            <Descriptions.Item label="Soft bounces">{stats.soft_bounces ?? 0}</Descriptions.Item>
            <Descriptions.Item label="Unsubscribed">{stats.unsubscribed ?? 0}</Descriptions.Item>
          </Descriptions>
          {stats.fetched_at && (
            <Text type="secondary" style={{ fontSize: 12 }}>
              Fetched from Mailchimp on {stats.fetched_at.replace('T', ' ').replace('Z', ' UTC')}.
            </Text>
          )}
        </>
      }
    />
  )
}
