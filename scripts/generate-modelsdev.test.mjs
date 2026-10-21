import { describe, expect, it } from 'bun:test'
import { convertCatalog, MODELS_DEV_API } from './generate-modelsdev.mjs'

describe('models.dev build-time metadata conversion', () => {
  const example = {
    openrouter: {
      name: 'OpenRouter',
      models: {
        'gpt-demo': { id: 'gpt-demo', name: 'Gateway GPT', reasoning: false },
      },
    },
    openai: {
      name: 'OpenAI',
      models: {
        'gpt-demo': {
          id: 'gpt-demo',
          name: 'GPT Demo',
          family: 'gpt',
          reasoning: true,
          tool_call: true,
          structured_output: true,
          limit: { context: 200000, output: 64000 },
          modalities: { input: ['text', 'image'], output: ['text'] },
          cost: { input: 1, output: 10 },
        },
        'old-model': { id: 'old-model', status: 'deprecated' },
      },
    },
  }

  it('prefers first-party provider for duplicate IDs', () => {
    const result = convertCatalog(example, '2026-10-21T00:00:00Z')
    const model = result.models.find(x => x.model_name === 'gpt-demo')
    expect(result.source_url).toBe(MODELS_DEV_API)
    expect(result.generated_at).toBe('2026-10-21T00:00:00Z')
    expect(result.models).toHaveLength(2)
    expect(model.vendor_name).toBe('OpenAI')
    expect(model.tags).toContain('reasoning')
    expect(model.tags).toContain('tool-call')
    expect(model.tags).toContain('input:image')
    expect(model.description).toContain('Context: 200000 tokens')
    expect(model.endpoints).toBe('')
    expect(model).not.toHaveProperty('cost')
    expect(result.models.find(x => x.model_name === 'old-model').status).toBe(0)
  })

  it('rejects malformed input and ignores invalid model records', () => {
    expect(() => convertCatalog(null)).toThrow()
    const result = convertCatalog({
      empty: { models: [] },
      openai: { name: 'OpenAI', models: {
        '': { id: '' },
        good: { id: 'good' },
        long: { id: 'x'.repeat(140) },
      } },
    })
    expect(result.models.map(m => m.model_name)).toEqual(['good'])
  })

  it('keeps conversion deterministic for a fixed timestamp', () => {
    const a = convertCatalog(example, '2026-10-21T00:00:00Z')
    const b = convertCatalog(example, '2026-10-21T00:00:00Z')
    expect(a).toEqual(b)
  })
})
