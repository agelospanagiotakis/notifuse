import { useEffect, useRef, useState } from 'react'
import { Button, Modal, Space, Switch, Tooltip, Typography, message } from 'antd'
import { Expand, Maximize, RotateCcw, Image as ImageIcon } from 'lucide-react'
import { Trans } from '@lingui/react/macro'

const { Text } = Typography

export interface Model3DViewerProps {
  open: boolean
  url: string
  name?: string
  onClose: () => void
}

/**
 * A GLB viewer that lets the model be rotated, zoomed and reset.
 *
 * The library is imported on demand: it is ~300 kB of renderer that most console sessions never
 * open, and a template author should not pay for it on every page load.
 */
export function Model3DViewer({ open, url, name, onClose }: Model3DViewerProps) {
  const viewerRef = useRef<HTMLElement | null>(null)
  const [ready, setReady] = useState(false)
  const [failed, setFailed] = useState<string | null>(null)
  const [autoRotate, setAutoRotate] = useState(false)
  const [loaded, setLoaded] = useState(false)

  useEffect(() => {
    if (!open) return
    let cancelled = false
    setLoaded(false)
    setFailed(null)
    setAutoRotate(false)

    import('@google/model-viewer')
      .then(() => {
        if (!cancelled) setReady(true)
      })
      .catch(() => {
        if (!cancelled) setFailed('The 3D viewer could not be loaded.')
      })

    return () => {
      cancelled = true
    }
  }, [open])

  useEffect(() => {
    const el = viewerRef.current
    if (!el) return
    const onLoad = () => setLoaded(true)
    const onError = () => {
      // The usual causes are a CORS refusal or a file that is not really a GLB. Saying which
      // is the difference between a five-minute fix and an afternoon.
      setFailed(
        'The model could not be loaded. Check that the file is a valid .glb and that the bucket allows this site to read it.'
      )
    }
    el.addEventListener('load', onLoad)
    el.addEventListener('error', onError)
    return () => {
      el.removeEventListener('load', onLoad)
      el.removeEventListener('error', onError)
    }
  }, [ready, url])

  const camera = () => {
    const el = viewerRef.current as unknown as
      | { getCameraOrbit?: () => { theta: number; phi: number; radius: number }; jumpCameraToGoal?: () => void }
      | null
    return el
  }

  const resetView = () => {
    const el = camera()
    if (!el?.getCameraOrbit) return
    // Back to the default framing: the front, at the model's own fit distance.
    el.getCameraOrbit()
    const anyEl = el as unknown as { resetTurntableRotation?: () => void }
    anyEl.resetTurntableRotation?.()
    el.jumpCameraToGoal?.()
  }

  const fullscreen = async () => {
    const el = viewerRef.current
    if (!el) return
    try {
      await el.requestFullscreen()
    } catch {
      message.error('This browser refused fullscreen for the viewer.')
    }
  }

  const copyLink = async () => {
    await navigator.clipboard.writeText(url)
    message.success(<Trans>Model URL copied</Trans>)
  }

  return (
    <Modal
      open={open}
      onCancel={onClose}
      footer={null}
      width={900}
      destroyOnHidden
      title={
        <Space>
          <Text strong>{name || '3D model'}</Text>
          <Tooltip title={<Trans>Drag to rotate · scroll to zoom</Trans>}>
            <Text type="secondary" style={{ fontWeight: 400 }}>
              <Trans>Drag to rotate · scroll to zoom</Trans>
            </Text>
          </Tooltip>
        </Space>
      }
    >
      <div
        style={{
          position: 'relative',
          width: '100%',
          height: 'min(62vh, 520px)',
          background:
            'radial-gradient(circle at 50% 35%, #f7f8fa 0%, #e7e9ee 55%, #d9dce3 100%)',
          borderRadius: 8,
          overflow: 'hidden'
        }}
      >
        {ready && !failed && (
          <model-viewer
            ref={(el: HTMLElement | null) => {
              viewerRef.current = el
            }}
            src={url}
            alt={name || '3D model'}
            camera-controls
            auto-rotate={autoRotate ? '' : undefined}
            interaction-prompt="auto"
            shadow-intensity="0.35"
            exposure="1"
            touch-action="pan-y"
            style={{ width: '100%', height: '100%', backgroundColor: 'transparent' }}
          />
        )}

        {!loaded && !failed && (
          <div style={{ position: 'absolute', inset: 0, display: 'grid', placeItems: 'center' }}>
            <Text type="secondary">
              <Trans>Loading model…</Trans>
            </Text>
          </div>
        )}

        {failed && (
          <div style={{ position: 'absolute', inset: 0, display: 'grid', placeItems: 'center', padding: 24 }}>
            <Text type="danger" style={{ textAlign: 'center' }}>
              {failed}
            </Text>
          </div>
        )}
      </div>

      <Space style={{ marginTop: 12, width: '100%', justifyContent: 'space-between' }} wrap>
        <Space>
          <Space size={6}>
            <RotateCcw size={15} />
            <Switch
              size="small"
              checked={autoRotate}
              onChange={setAutoRotate}
              aria-label="Auto-rotate"
            />
            <Text type="secondary">
              <Trans>Auto-rotate</Trans>
            </Text>
          </Space>
          <Tooltip title={<Trans>Reset the camera</Trans>}>
            <Button icon={<Expand size={15} />} onClick={resetView}>
              <Trans>Reset view</Trans>
            </Button>
          </Tooltip>
          <Tooltip title={<Trans>Fullscreen</Trans>}>
            <Button icon={<Maximize size={15} />} onClick={fullscreen} />
          </Tooltip>
        </Space>
        <Space>
          <Tooltip title={<Trans>Copy the model URL</Trans>}>
            <Button icon={<ImageIcon size={15} />} onClick={copyLink}>
              <Trans>Copy URL</Trans>
            </Button>
          </Tooltip>
          <Button type="primary" onClick={onClose}>
            <Trans>Close</Trans>
          </Button>
        </Space>
      </Space>
    </Modal>
  )
}
