import { llmSettingDefaults, resolveInitialLlmSetting } from './llm-setting-defaults';

// The four generation parameters must fall back to the backend defaults
// (LLM_SETTING_DEFAULTS in api/db/services/llm_service.py), not to 0: an
// unset top_p persisted as 0 would collapse nucleus sampling on the next
// save because the enabled flags default to true.

describe('resolveInitialLlmSetting', () => {
  test('unset llm_setting falls back to the backend defaults', () => {
    expect(resolveInitialLlmSetting(undefined)).toEqual(llmSettingDefaults);
    expect(resolveInitialLlmSetting(null)).toEqual(llmSettingDefaults);
    expect(resolveInitialLlmSetting({})).toEqual(llmSettingDefaults);
  });

  test('stored values are kept', () => {
    expect(
      resolveInitialLlmSetting({
        temperature: 0.8,
        top_p: 0.9,
        frequency_penalty: 0.2,
        presence_penalty: 0.5,
      }),
    ).toEqual({
      temperature: 0.8,
      top_p: 0.9,
      frequency_penalty: 0.2,
      presence_penalty: 0.5,
    });
  });

  test('a stored 0 is a deliberate choice and must not be replaced', () => {
    expect(
      resolveInitialLlmSetting({
        temperature: 0,
        top_p: 0,
        frequency_penalty: 0,
        presence_penalty: 0,
      }),
    ).toEqual({
      temperature: 0,
      top_p: 0,
      frequency_penalty: 0,
      presence_penalty: 0,
    });
  });

  test('partially stored values only fill the missing ones', () => {
    expect(resolveInitialLlmSetting({ temperature: 0.5 })).toEqual({
      temperature: 0.5,
      top_p: llmSettingDefaults.top_p,
      frequency_penalty: llmSettingDefaults.frequency_penalty,
      presence_penalty: llmSettingDefaults.presence_penalty,
    });
  });
});
