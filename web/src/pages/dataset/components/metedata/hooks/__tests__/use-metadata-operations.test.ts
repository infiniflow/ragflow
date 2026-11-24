import { act, renderHook } from '@testing-library/react';
import { useMetadataOperations } from '../use-manage-modal';

describe('delete metadata and add a list in the same save', () => {
  it.each(['status', 'tags'])('preserves the new list field %s', (newKey) => {
    const { result } = renderHook(useMetadataOperations);
    act(() => {
      result.current.addDeleteRow('status');
      result.current.addUpdateValue(newKey, '', ['first', 'second'], 'list');
    });

    expect(result.current.operations).toEqual({
      updates: [
        {
          key: newKey,
          match: '',
          value: ['first', 'second'],
          valueType: 'list',
        },
      ],
      deletes: newKey === 'status' ? [] : [{ key: 'status' }],
    });
  });

  it('keeps a field deletion made after adding the new list', () => {
    const { result } = renderHook(useMetadataOperations);
    act(() => {
      result.current.addUpdateValue('status', '', ['first', 'second'], 'list');
      result.current.addDeleteRow('status');
    });
    expect(result.current.operations.deletes).toEqual([{ key: 'status' }]);
  });
});
