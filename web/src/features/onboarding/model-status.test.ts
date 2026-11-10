import { expect, it } from 'vitest'
import { isStoryModelReady } from './model-status'
import type { LayeredSettings } from '@/features/settings/types'

function layered(runtime: LayeredSettings['runtime'], effective: LayeredSettings['effective'] = {}): LayeredSettings {
  return {
    default: {}, global: {}, user: {}, workspace: {}, effective,
    paths: { denova_dir: '', nova_dir: '', user_config: '', workspace_config: '' },
    runtime, resolved_agent_tool_manifests: {}, resolved_agent_contexts: {},
  }
}

it("takes the backend's answer over the settings-layer inference", () => {
  // Environment overrides and keyless local providers make a working machine
  // look unconfigured in the persisted layers; the backend resolves the route a
  // real call uses, so its answer wins.
  expect(isStoryModelReady(layered({ goos: 'windows', story_model_ready: true }))).toBe(true)
  expect(isStoryModelReady(layered({ goos: 'windows', story_model_ready: false }, {
    openai_api_key: 'test-key', openai_model: 'my-model',
  }))).toBe(false)
})

it('falls back to the inference when the backend has not answered', () => {
  expect(isStoryModelReady(layered({ goos: 'windows' }))).toBe(false)
  expect(isStoryModelReady(layered({ goos: 'windows' }, {
    model_endpoints: [{ id: 'mine', api_key: 'test-key', base_url: 'https://models.example/v1' }],
    model_profiles: [{ id: 'my-model', endpoint_id: 'mine', model: 'my-model' }],
  }))).toBe(true)
  expect(isStoryModelReady(undefined)).toBe(false)
})
