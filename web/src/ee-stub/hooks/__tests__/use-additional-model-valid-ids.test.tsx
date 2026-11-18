import { act, renderHook } from '@testing-library/react';
import { useAdditionalModelValidIds } from '../use-additional-model-valid-ids';

const groupRef = 'gp-0123456789abcdef0123456789abcdef';
const directModel = {
  model_id: 'model-1',
  status: 'active',
  instance_status: 'active',
  model_type: ['chat'],
  name: 'gpt-4',
  provider_id: 'provider-1',
  provider_name: 'OpenAI',
  instance_id: 'instance-1',
  instance_name: 'default',
};

describe('OSS useAdditionalModelValidIds', () => {
  it('keeps group ids empty and follows only direct model readiness', async () => {
    const { result, rerender } = renderHook(
      ({ modelsFetched }) =>
        useAdditionalModelValidIds({
          modelTypes: ['chat'],
          models: [directModel],
          modelsFetched,
          enabled: true,
        }),
      { initialProps: { modelsFetched: false } },
    );

    expect(result.current.isSettled).toBe(false);
    expect(result.current.validIds).toEqual(new Set());
    expect(result.current.getPendingMessage(groupRef)).toBeUndefined();

    rerender({ modelsFetched: true });
    expect(result.current.isSettled).toBe(true);
    expect(result.current.validIds).toEqual(new Set());
    expect(result.current.getPendingMessage(groupRef)).toBeUndefined();

    await act(async () => {
      await expect(
        result.current.resolveValidIds([directModel]),
      ).resolves.toEqual(new Set());
    });
  });
});
