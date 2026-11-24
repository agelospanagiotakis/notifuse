import { afterEach, describe, expect, it, vi } from 'vitest'
import { handOffSignInEmail, takeSignInEmail } from './signInHandoff'

describe('signInHandoff', () => {
  afterEach(() => {
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
    sessionStorage.clear()
  })

  it('gives the handed email once', () => {
    handOffSignInEmail('admin@acme.com')
    expect(takeSignInEmail()).toBe('admin@acme.com')
    expect(takeSignInEmail()).toBeUndefined()
  })

  it('stores nothing for no email', () => {
    handOffSignInEmail(undefined)
    expect(takeSignInEmail()).toBeUndefined()
  })

  it('does without storage when the browser refuses it', () => {
    vi.stubGlobal('sessionStorage', {
      setItem: () => {
        throw new Error('blocked')
      },
      getItem: () => {
        throw new Error('blocked')
      },
      removeItem: () => {
        throw new Error('blocked')
      }
    })
    expect(() => handOffSignInEmail('admin@acme.com')).not.toThrow()
    expect(takeSignInEmail()).toBeUndefined()
  })
})
