import { Operator } from '@/constants/agent';
import { findUnavailableCanvasResource } from './find-unavailable-resource';

const node = (label: Operator, form: Record<string, unknown>) =>
  ({ data: { label, name: 'node', form } }) as any;

describe('findUnavailableCanvasResource', () => {
  it('detects missing resources without flagging available ones', () => {
    const modelNode = node(Operator.Agent, { llm_id: 'missing' });
    const compilerNode = node(Operator.Compiler, {
      llm_id: 'model-id',
      compilation_template_group_id: 'group-id',
    });

    expect(findUnavailableCanvasResource([modelNode], [], [])?.messageKey).toBe(
      'common.modelUnavailable',
    );
    expect(
      findUnavailableCanvasResource([modelNode], undefined, [])?.messageKey,
    ).toBe('flow.canvasResourcesUnavailable');
    expect(
      findUnavailableCanvasResource(
        [compilerNode],
        [{ model_id: 'model-id' }] as any,
        [],
      )?.messageKey,
    ).toBe('flow.compilationOperatorMissing');
    expect(
      findUnavailableCanvasResource(
        [compilerNode],
        [{ model_id: 'model-id' }] as any,
        [{ id: 'group-id' }] as any,
      ),
    ).toBeUndefined();
    expect(
      findUnavailableCanvasResource(
        [node(Operator.Parser, { parse_method: 'DeepDOC' })],
        undefined,
        undefined,
      ),
    ).toBeUndefined();
  });
});
