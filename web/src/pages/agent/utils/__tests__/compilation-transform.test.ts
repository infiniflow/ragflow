import { Operator } from '@/constants/agent';
import { transformFormConfigToApi } from '@/utils/pipeline-operator';
import { transformCompilationParams } from '../../utils';

describe('Compiler parameter transformations', () => {
  it('emits the compilation template group plus the LLM runtime settings', () => {
    const input: any = {
      compilation_template_group_id: 'group-1',
      llm_id: 'model-1',
      temperature: 0.5,
      temperatureEnabled: true,
      top_p: 0.9,
      topPEnabled: true,
      presence_penalty: 0.2,
      presencePenaltyEnabled: true,
      frequency_penalty: 0.3,
      frequencyPenaltyEnabled: true,
      max_tokens: 1024,
      maxTokensEnabled: true,
      parameter: 'Custom',
      thinking: 'enabled',
      // Fields that must not leak into the DSL params
      outputs: { chunks: { type: 'Array<Object>', value: [] } },
      mode: 'structure',
    };

    const result = transformCompilationParams(input);

    expect(result).toEqual({
      compilation_template_group_id: 'group-1',
      llm_id: 'model-1',
      temperature: 0.5,
      temperatureEnabled: true,
      top_p: 0.9,
      topPEnabled: true,
      presence_penalty: 0.2,
      presencePenaltyEnabled: true,
      frequency_penalty: 0.3,
      frequencyPenaltyEnabled: true,
      max_tokens: 1024,
      maxTokensEnabled: true,
      parameter: 'Custom',
      thinking: 'enabled',
    });
  });

  it('is wired into the document pipeline save path', () => {
    const result = transformFormConfigToApi(Operator.Compiler, {
      compilation_template_group_id: 'group-1',
      llm_id: 'model-1',
      temperature: 0.7,
      temperatureEnabled: true,
      outputs: {},
    });

    expect(result).toEqual({
      compilation_template_group_id: 'group-1',
      llm_id: 'model-1',
      temperature: 0.7,
      temperatureEnabled: true,
    });
  });
});
