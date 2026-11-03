import { describe, it, expect } from 'vitest'
import { llmProviders, getLLMProviderName, getLLMProviderIcon } from './LLMProviders'

describe('LLM provider catalogue', () => {
  it('offers DeepSeek as its own provider, not as an OpenAI preset', () => {
    const deepseek = llmProviders.find((p) => p.kind === 'deepseek')
    expect(deepseek).toBeDefined()
    expect(deepseek?.name).toBe('DeepSeek')
    expect(deepseek?.defaultModel).toBe('deepseek-chat')
    // A distinct kind is the point: folded into OpenAI, an operator would never find it.
    expect(llmProviders.filter((p) => p.kind === 'openai')).toHaveLength(1)
  })

  it('names every provider the server accepts', () => {
    for (const kind of ['anthropic', 'openai', 'gemini', 'deepseek']) {
      expect(getLLMProviderName(kind)).not.toBe(kind)
    }
    expect(getLLMProviderName('deepseek')).toBe('DeepSeek')
  })

  it('renders an icon for DeepSeek', () => {
    expect(getLLMProviderIcon('deepseek')).toBeTruthy()
  })
})
