import { describe, it, expect } from 'vitest'
import { TEMPLATE_STARTERS, KOIS_BRAND } from './starters'
import type { EmailBlock } from '../email_builder/types'

const flatten = (b: EmailBlock, out: EmailBlock[] = []): EmailBlock[] => {
  out.push(b)
  const children = (b as unknown as { children?: EmailBlock[] }).children ?? []
  for (const c of children) flatten(c, out)
  return out
}

const serialise = (tree: EmailBlock) => JSON.stringify(tree)

describe('brand starters', () => {
  it('offers four designs, each with a name and a description', () => {
    expect(TEMPLATE_STARTERS).toHaveLength(4)
    for (const s of TEMPLATE_STARTERS) {
      expect(s.id).toBeTruthy()
      expect(s.name).toBeTruthy()
      expect(s.description).toBeTruthy()
      expect(['marketing', 'transactional']).toContain(s.category)
    }
  })

  it('uses ids that are unique inside a tree', () => {
    // Duplicate ids would silently make two blocks select and edit as one.
    for (const s of TEMPLATE_STARTERS) {
      const ids = flatten(s.tree).map((b) => (b as unknown as { id: string }).id)
      expect(new Set(ids).size, `${s.id} has duplicate block ids`).toBe(ids.length)
      expect(ids.every((id) => typeof id === 'string' && id.length > 0)).toBe(true)
    }
  })

  it('carries the brand wordmark and the platform liquid variables', () => {
    for (const s of TEMPLATE_STARTERS) {
      const json = serialise(s.tree)
      expect(json, `${s.id} is missing the logo`).toContain(KOIS_BRAND.logo)
      // Notifuse's own variables, never Mailchimp's merge tags.
      expect(json, `${s.id} cannot unsubscribe`).toContain('{{ unsubscribe_url }}')
      expect(json).not.toContain('*|')
    }
  })

  it('keeps a head with a title and the inbox preview block', () => {
    for (const s of TEMPLATE_STARTERS) {
      const json = serialise(s.tree)
      expect(json).toContain('mj-title')
      expect(json).toContain('{{ subject_preview }}')
    }
  })

  it('is JSON-cloneable, because the drawer deep-clones it per use', () => {
    // The editor must not share one object between templates opened in the same session.
    for (const s of TEMPLATE_STARTERS) {
      const clone = JSON.parse(JSON.stringify(s.tree))
      expect(clone).toEqual(s.tree)
      expect(clone).not.toBe(s.tree)
    }
  })
})
