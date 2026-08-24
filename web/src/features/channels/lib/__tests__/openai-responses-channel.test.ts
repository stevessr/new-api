/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { describe, expect, test } from 'vitest'

import {
  CHANNEL_TYPE_OPENAI_RESPONSE,
  CHANNEL_TYPE_OPTIONS,
  FIELD_PASSTHROUGH_TYPES,
  MODEL_FETCHABLE_TYPES,
  OPENAI_FIELD_PASSTHROUGH_TYPES,
  OPENAI_NATIVE_CHANNEL_TYPES,
} from '../../constants'
import { CHANNEL_FORM_DEFAULT_VALUES, channelFormSchema } from '../channel-form'
import {
  getChannelTypeConfig,
  requiresOrganization,
} from '../channel-type-config'
import { getChannelTypeIcon, getKeyPromptForType } from '../channel-utils'

describe('OpenAI Responses channel', () => {
  test('registers the dedicated channel next to OpenAI with Responses capabilities', () => {
    const option = CHANNEL_TYPE_OPTIONS.find(
      (item) => item.value === CHANNEL_TYPE_OPENAI_RESPONSE
    )

    expect(option).toEqual({
      value: CHANNEL_TYPE_OPENAI_RESPONSE,
      label: 'OpenAI Responses',
    })
    expect(
      CHANNEL_TYPE_OPTIONS.findIndex(
        (item) => item.value === CHANNEL_TYPE_OPENAI_RESPONSE
      )
    ).toBe(CHANNEL_TYPE_OPTIONS.findIndex((item) => item.value === 1) + 1)
    expect(MODEL_FETCHABLE_TYPES.has(CHANNEL_TYPE_OPENAI_RESPONSE)).toBe(true)
    expect(FIELD_PASSTHROUGH_TYPES.has(CHANNEL_TYPE_OPENAI_RESPONSE)).toBe(true)
    expect(
      OPENAI_FIELD_PASSTHROUGH_TYPES.has(CHANNEL_TYPE_OPENAI_RESPONSE)
    ).toBe(true)
    expect(OPENAI_NATIVE_CHANNEL_TYPES.has(CHANNEL_TYPE_OPENAI_RESPONSE)).toBe(
      true
    )
  })

  test('uses OpenAI defaults without requiring a custom Base URL', () => {
    const config = getChannelTypeConfig(CHANNEL_TYPE_OPENAI_RESPONSE)

    expect(config.defaultBaseUrl).toBe('https://api.openai.com')
    expect(config.icon).toBe('openai')
    expect(requiresOrganization(CHANNEL_TYPE_OPENAI_RESPONSE)).toBe(true)
    expect(getChannelTypeIcon(CHANNEL_TYPE_OPENAI_RESPONSE)).toBe('OpenAI')
    expect(getKeyPromptForType(CHANNEL_TYPE_OPENAI_RESPONSE)).toBe(
      'Enter API key for this channel'
    )
    expect(
      channelFormSchema.safeParse({
        ...CHANNEL_FORM_DEFAULT_VALUES,
        name: 'Responses upstream',
        type: CHANNEL_TYPE_OPENAI_RESPONSE,
        base_url: '',
        key: 'responses-key',
        models: 'gpt-5',
      }).success
    ).toBe(true)
  })
})
