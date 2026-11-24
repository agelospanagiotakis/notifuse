import { render } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { NotifuseLogo, NotifuseMark } from './NotifuseLogo'

// The console is white-labelled: the deployment overlays /console/logo.png and icon.png, so the
// components must render those images rather than an embedded Notifuse drawing.
describe('brand images', () => {
  it('the wordmark is the deployed logo image', () => {
    const { container } = render(<NotifuseLogo />)
    const img = container.querySelector('img')
    expect(img).not.toBeNull()
    expect(img?.getAttribute('src')).toBe('/console/logo.png')
  })

  it('the symbol is the deployed icon image', () => {
    const { container } = render(<NotifuseMark />)
    const img = container.querySelector('img')
    expect(img).not.toBeNull()
    expect(img?.getAttribute('src')).toBe('/console/icon.png')
  })
})
