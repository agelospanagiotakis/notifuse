import { Form, Input, Button, Card, App, Typography } from 'antd'
import { useState } from 'react'
import { useLingui } from '@lingui/react/macro'
import { authService } from '../../services/api/auth'

const { Title, Paragraph } = Typography

/**
 * Set or change the password of the signed-in user (e-Pragma fork).
 *
 * Accounts created with magic codes have no password; the first save adopts one. Once set,
 * the sign-in screen offers "Sign in with a password", and the emailed code keeps working
 * as before, so nobody is locked out by this setting.
 */
export function SecuritySettings() {
  const { t } = useLingui()
  const [form] = Form.useForm()
  const { message } = App.useApp()
  const [saving, setSaving] = useState(false)

  const handleFinish = async (values: {
    current_password?: string
    new_password: string
  }) => {
    try {
      setSaving(true)
      await authService.setPassword({
        current_password: values.current_password || '',
        new_password: values.new_password
      })
      message.success(t`Password saved. You can now sign in with it.`)
      form.resetFields()
    } catch (error) {
      message.error(error instanceof Error ? error.message : t`Failed to save the password`)
    } finally {
      setSaving(false)
    }
  }

  return (
    <Card>
      <Title level={4} style={{ marginTop: 0 }}>{t`Password sign-in`}</Title>
      <Paragraph type="secondary">
        {t`Set a password to sign in without waiting for an emailed code. Leave it unset and the magic code keeps working.`}
      </Paragraph>

      <Form form={form} layout="vertical" onFinish={handleFinish} style={{ maxWidth: 420 }}>
        <Form.Item
          label={t`Current password`}
          name="current_password"
          extra={t`Leave empty if you have never set one.`}
        >
          <Input.Password autoComplete="current-password" />
        </Form.Item>

        <Form.Item
          label={t`New password`}
          name="new_password"
          rules={[
            { required: true, message: t`Please enter a new password` },
            { min: 8, message: t`Use at least 8 characters` }
          ]}
        >
          <Input.Password autoComplete="new-password" />
        </Form.Item>

        <Form.Item
          label={t`Confirm new password`}
          name="confirm"
          dependencies={['new_password']}
          rules={[
            { required: true, message: t`Please confirm the password` },
            ({ getFieldValue }) => ({
              validator(_, value) {
                if (!value || getFieldValue('new_password') === value) return Promise.resolve()
                return Promise.reject(new Error(t`The passwords do not match`))
              }
            })
          ]}
        >
          <Input.Password autoComplete="new-password" />
        </Form.Item>

        <Form.Item>
          <Button type="primary" htmlType="submit" loading={saving}>
            {t`Save password`}
          </Button>
        </Form.Item>
      </Form>
    </Card>
  )
}
