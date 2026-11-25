import { act, renderHook } from '@testing-library/react';
import type { MetadataValueType } from '../../interface';
import { useMetadataOperations } from '../use-manage-modal';

describe.each<[MetadataValueType, string[], string | string[]]>([
  ['list', ['first', 'second'], ['first', 'second']],
  ['string', ['published'], 'published'],
  ['time', ['2026-11-24 10:30:00'], '2026-11-24 10:30:00'],
  ['number', ['42'], '42'],
])('delete and add a %s metadata field', (valueType, values, value) => {
  it.each(['status', 'replacement'])('preserves the new field %s', (newKey) => {
    const { result } = renderHook(useMetadataOperations);
    act(() => {
      result.current.addDeleteRow('status');
      result.current.addUpdateValue(newKey, '', values, valueType);
    });

    expect(result.current.operations).toEqual({
      updates: [
        {
          key: newKey,
          match: '',
          value,
          valueType,
        },
      ],
      deletes: newKey === 'status' ? [] : [{ key: 'status' }],
    });
  });

  it('keeps a field deletion made after the addition', () => {
    const { result } = renderHook(useMetadataOperations);
    act(() => {
      result.current.addUpdateValue('status', '', values, valueType);
      result.current.addDeleteRow('status');
    });
    expect(result.current.operations.deletes).toEqual([{ key: 'status' }]);
  });
});
