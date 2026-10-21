// Build-time snapshot generator for https://models.dev/api.json.
// Run: bun scripts/generate-modelsdev.mjs [--input catalog.json] [--output controller/modelsdev_snapshot.json]
// The snapshot contains descriptive metadata only; provider-specific pricing is deliberately not imported.
import { readFile, writeFile, rename } from 'node:fs/promises'
import { dirname, resolve } from 'node:path'
import { mkdir } from 'node:fs/promises'

export const MODELS_DEV_API = 'https://models.dev/api.json'

const preferredProviders = new Set([
  'openai', 'anthropic', 'google', 'deepseek', 'xai', 'mistral', 'cohere',
  'meta', 'qwen', 'alibaba', 'zai', 'zhipuai', 'moonshotai', 'perplexity',
  'groq', 'nvidia', 'amazon-bedrock', 'azure', 'baidu', 'minimax',
])

const safeText = (value, max = 600) =>
  typeof value === 'string' ? value.trim().slice(0, max) : ''

const positiveInteger = (value) =>
  Number.isSafeInteger(value) && value > 0 ? value : null

export function convertCatalog(catalog, generatedAt = new Date().toISOString()) {
  if (!catalog || typeof catalog !== 'object' || Array.isArray(catalog)) {
    throw new Error('models.dev catalog must be an object of providers')
  }

  const chosen = new Map()
  const vendors = new Map()
  for (const [providerID, provider] of Object.entries(catalog).sort(([a], [b]) => a.localeCompare(b))) {
    if (!provider || typeof provider !== 'object' || Array.isArray(provider) ||
        !provider.models || typeof provider.models !== 'object' || Array.isArray(provider.models)) {
      continue
    }
    const vendorName = safeText(provider.name, 80) || providerID
    for (const [entryID, record] of Object.entries(provider.models)) {
      if (!record || typeof record !== 'object' || Array.isArray(record)) continue
      const modelID = safeText(record.id || entryID, 512)
      // New API model identifiers must fit its models.model_name varchar(128).
      if (!modelID || modelID.length > 128 || /[\u0000-\u001f]/.test(modelID)) continue

      const input = Array.isArray(record.modalities?.input) ? record.modalities.input : []
      const output = Array.isArray(record.modalities?.output) ? record.modalities.output : []
      const features = [
        record.reasoning === true && 'reasoning',
        record.tool_call === true && 'tool-call',
        record.structured_output === true && 'structured-output',
        ...input.filter(x => ['image', 'audio', 'video', 'pdf'].includes(x)).map(x => 'input:' + x),
        ...output.filter(x => ['image', 'audio', 'video'].includes(x)).map(x => 'output:' + x),
      ].filter(Boolean)
      const tags = [...new Set(['models.dev', ...features])].join(',')
      const details = [
        safeText(record.name, 160),
        safeText(record.family, 100) && 'Family: ' + safeText(record.family, 100),
        positiveInteger(record.limit?.context) && 'Context: ' + record.limit.context + ' tokens',
        positiveInteger(record.limit?.output) && 'Max output: ' + record.limit.output + ' tokens',
        safeText(record.knowledge, 40) && 'Knowledge: ' + safeText(record.knowledge, 40),
      ].filter(Boolean)
      const providerLabel = typeof record.provider === 'string'
        ? record.provider : safeText(record.provider?.id, 80)
      // On collisions prefer the model's own provider; provider gateways are
      // never treated as authoritative over first-party entries.
      const score = (providerLabel === providerID ? 1000 : 0) +
        (preferredProviders.has(providerID) ? 100 : 0) +
        (modelID.toLowerCase().startsWith(providerID.toLowerCase() + '/') ? 10 : 0)
      const candidate = {
        score, providerID,
        model: {
          model_name: modelID,
          description: details.join(' · ').slice(0, 700),
          icon: '',
          tags: tags.slice(0, 255),
          vendor_name: vendorName,
          endpoints: '',
          name_rule: 0,
          status: record.status === 'deprecated' ? 0 : 1,
        },
      }
      const old = chosen.get(modelID)
      if (!old || score > old.score || (score === old.score && providerID < old.providerID)) {
        chosen.set(modelID, candidate)
      }
      vendors.set(providerID, { name: vendorName, description: '', icon: '', status: 1 })
    }
  }

  const models = [...chosen.values()].map(v => v.model).sort((a,b)=>a.model_name.localeCompare(b.model_name))
  const used = new Set(models.map(m => m.vendor_name))
  // Multiple provider entries can have the same display name; keep one
  // vendor record per name so the Go-side sync validation stays unambiguous.
  const vendorList = [...new Map(
    [...vendors.values()].filter(v => used.has(v.name)).map(v => [v.name, v]),
  ).values()].sort((a,b)=>a.name.localeCompare(b.name))
  return {
    source_url: MODELS_DEV_API,
    generated_at: generatedAt,
    models,
    vendors: vendorList,
  }
}

export async function main(args = process.argv.slice(2)) {
  let input = null
  let output = 'controller/modelsdev_snapshot.json'
  for (let i = 0; i < args.length; i++) {
    if (args[i] === '--input' && args[i + 1]) input = args[++i]
    else if (args[i] === '--output' && args[i + 1]) output = args[++i]
    else throw new Error('Unexpected generator argument: ' + args[i])
  }
  if (process.env.MODELSDEV_SKIP_FETCH === '1' && !input) {
    console.log('models.dev download explicitly skipped; retaining existing snapshot')
    return
  }
  let raw
  if (input) {
    raw = await readFile(resolve(input), 'utf8')
  } else {
    const response = await fetch(MODELS_DEV_API, {
      signal: AbortSignal.timeout(20000),
      headers: { accept: 'application/json' },
    })
    if (!response.ok) throw new Error('models.dev returned HTTP ' + response.status)
    const length = Number(response.headers.get('content-length') || 0)
    if (length > 30 * 1024 * 1024) throw new Error('models.dev catalog exceeds 30 MiB')
    raw = await response.text()
  }
  if (raw.length > 30 * 1024 * 1024) throw new Error('models.dev catalog exceeds 30 MiB')
  const snapshot = convertCatalog(JSON.parse(raw))
  if (!input && snapshot.models.length < 100) {
    throw new Error('models.dev returned fewer than 100 models; refusing to overwrite snapshot')
  }
  if (!snapshot.models.length) throw new Error('No usable models in catalog')
  const target = resolve(output)
  await mkdir(dirname(target), { recursive: true })
  const temp = target + '.tmp'
  await writeFile(temp, JSON.stringify(snapshot) + '\n', 'utf8')
  await rename(temp, target)
  console.log('Bundled ' + snapshot.models.length + ' models / ' + snapshot.vendors.length + ' vendors from models.dev')
}
if (import.meta.main) await main()
