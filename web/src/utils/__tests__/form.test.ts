import {
  ModelVariableType,
  settledModelVariableMap,
} from '@/constants/knowledge';
import { deriveModelPreset, removeUselessFieldsFromValues } from '../form';

describe('deriveModelPreset', () => {
  it('derives the preset whose values are stored', () => {
    expect(
      deriveModelPreset({
        ...settledModelVariableMap[ModelVariableType.Precise],
      }),
    ).toBe(ModelVariableType.Precise);
    expect(
      deriveModelPreset({
        ...settledModelVariableMap[ModelVariableType.Balance],
      }),
    ).toBe(ModelVariableType.Balance);
    expect(
      deriveModelPreset({
        ...settledModelVariableMap[ModelVariableType.Improvise],
      }),
    ).toBe(ModelVariableType.Improvise);
  });

  it('matches a preset even when max_tokens was stripped on save', () => {
    // Presets force maxTokensEnabled off, so the persisted config lacks it.
    const stored = {
      temperature: 0.2,
      top_p: 0.75,
      frequency_penalty: 0.5,
      presence_penalty: 0.5,
    };
    expect(deriveModelPreset(stored)).toBe(ModelVariableType.Precise);
  });

  it('reports custom when a stored value diverges from every preset', () => {
    expect(
      deriveModelPreset({
        ...settledModelVariableMap[ModelVariableType.Precise],
        temperature: 0.3,
      }),
    ).toBe('Custom');
  });

  it('reports custom when max_tokens diverges from the matching preset', () => {
    expect(
      deriveModelPreset({
        ...settledModelVariableMap[ModelVariableType.Balance],
        max_tokens: 8192,
      }),
    ).toBe('Custom');
  });

  it('reports custom when only the ambiguous max_tokens is stored', () => {
    // Every preset uses 4096, so no unique match is possible.
    expect(deriveModelPreset({ max_tokens: 4096 })).toBe('Custom');
  });

  it('reports custom for an empty or missing llm_setting', () => {
    expect(deriveModelPreset(undefined)).toBe('Custom');
    expect(deriveModelPreset({})).toBe('Custom');
  });
});

describe('removeUselessFieldsFromValues', () => {
  it('strips the preset name at the given prefix', () => {
    expect(
      removeUselessFieldsFromValues(
        {
          llm_setting: { parameter: 'Precise', temperature: 0.2 },
          temperatureEnabled: true,
        },
        'llm_setting.',
      ),
    ).toEqual({ llm_setting: { temperature: 0.2 } });
  });

  it('strips the preset name and disabled variables from flat values', () => {
    expect(
      removeUselessFieldsFromValues(
        {
          parameter: 'Precise',
          temperature: 0.2,
          max_tokens: 4096,
          temperatureEnabled: true,
          maxTokensEnabled: false,
        },
        '',
      ),
    ).toEqual({ temperature: 0.2 });
  });
});
