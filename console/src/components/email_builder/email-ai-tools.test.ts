import { describe, it, expect } from 'vitest'
import { serializeEmailTree } from './email-ai-tools'
import type { EmailBlock } from './types'

const tree = {
  id: 'root',
  type: 'mjml',
  children: [
    {
      id: 'body-1',
      type: 'mj-body',
      children: [
        {
          id: 'sec-1',
          type: 'mj-section',
          children: [
            {
              id: 'col-1',
              type: 'mj-column',
              children: [
                { id: 'txt-1', type: 'mj-text', content: '<p>Μοντέλο πρώτο</p>' },
                {
                  id: 'img-1',
                  type: 'mj-image',
                  attributes: {
                    src: 'https://minio-api.e-pragma.gr/koisnewsletters/newsletters/x/dior-clover.png',
                    alt: 'Dior Clover'
                  }
                }
              ]
            }
          ]
        }
      ]
    }
  ]
} as unknown as EmailBlock

describe('serializeEmailTree', () => {
  it('shows the image src, so "change the photos" has something to act on', () => {
    // It used to render "src: ..." — the model knew an image existed but not which one.
    const summary = serializeEmailTree(tree)
    expect(summary).toContain('dior-clover.png')
    expect(summary).not.toContain('src: ...')
  })

  it('shows the alt text and the block ids the tools need', () => {
    const summary = serializeEmailTree(tree)
    expect(summary).toContain('alt: "Dior Clover"')
    expect(summary).toContain('img-1')
    expect(summary).toContain('txt-1')
  })

  it('strips HTML from text content but keeps the words', () => {
    const summary = serializeEmailTree(tree)
    expect(summary).toContain('content: "Μοντέλο πρώτο"')
    expect(summary).not.toContain('<p>')
  })

  it('shortens a very long signed URL without losing the filename', () => {
    const long = 'https://cdn.example.com/' + 'a'.repeat(200) + '/product-photo.jpg'
    const t = {
      id: 'r',
      type: 'mjml',
      children: [{ id: 'i', type: 'mj-image', attributes: { src: long } }]
    } as unknown as EmailBlock
    const summary = serializeEmailTree(t)

    expect(summary).toContain('product-photo.jpg')
    expect(summary.length).toBeLessThan(long.length)
  })
})
