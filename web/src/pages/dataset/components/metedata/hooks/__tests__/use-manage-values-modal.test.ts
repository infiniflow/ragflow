jest.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

import { act, renderHook } from '@testing-library/react';
import { MetadataType, metadataValueTypeEnum } from '../../constant';
import { IManageValuesProps, IMetaDataTableData } from '../../interface';
import { useManageValues } from '../use-manage-values-modal';

function makeProps(
  overrides: Partial<IManageValuesProps> = {},
): IManageValuesProps {
  const data: IMetaDataTableData = {
    field: '',
    description: '',
    values: [''],
    valueType: metadataValueTypeEnum.string,
  };
  return {
    title: '',
    visible: true,
    type: MetadataType.UpdateSingle,
    existsKeys: [],
    isEditField: true,
    isAddValue: true,
    isAddValueMode: true,
    isShowDescription: false,
    isShowValueSwitch: false,
    isShowType: true,
    isVerticalShowValue: true,
    data,
    onSave: jest.fn(),
    hideModal: jest.fn(),
    addUpdateValue: jest.fn(),
    addDeleteValue: jest.fn(),
    ...overrides,
  } as IManageValuesProps;
}

describe('useManageValues - add new metadata', () => {
  it('queues the typed value, not the pre-blur empty string', () => {
    const addUpdateValue = jest.fn();
    const props = makeProps({ addUpdateValue });

    const { result } = renderHook(() => useManageValues(props));

    act(() => {
      result.current.handleChange('field', 'user_name');
      result.current.handleValueChange(0, 'Employee', false);
    });

    expect(result.current.metaData.values).toEqual(['']);
    expect(result.current.tempValues).toEqual(['Employee']);

    act(() => {
      result.current.handleSave();
    });

    expect(addUpdateValue).toHaveBeenCalledTimes(1);
    const [, , queuedValues] = addUpdateValue.mock.calls[0];
    expect(queuedValues).toEqual(['Employee']);
  });

  it('still passes the typed value when the blur did fire first', () => {
    const addUpdateValue = jest.fn();
    const props = makeProps({ addUpdateValue });

    const { result } = renderHook(() => useManageValues(props));

    act(() => {
      result.current.handleChange('field', 'user_name');
      result.current.handleValueChange(0, 'Employee', false);
      result.current.handleValueBlur();
    });

    act(() => {
      result.current.handleSave();
    });

    expect(addUpdateValue).toHaveBeenCalledTimes(1);
    const [, , queuedValues] = addUpdateValue.mock.calls[0];
    expect(queuedValues).toEqual(['Employee']);
  });
});

describe('useManageValues - duplicate field name guard', () => {
  it('blocks save when field name already exists in Setting mode', () => {
    const addUpdateValue = jest.fn();
    const onSave = jest.fn();
    const props = makeProps({
      type: MetadataType.Setting,
      existsKeys: ['author'],
      addUpdateValue,
      onSave,
    });

    const { result } = renderHook(() => useManageValues(props));

    act(() => {
      result.current.handleChange('field', 'author');
      result.current.handleValueChange(0, 'Alice', false);
    });

    expect(result.current.valueError.field).not.toBe('');

    act(() => {
      result.current.handleSave();
    });

    expect(addUpdateValue).not.toHaveBeenCalled();
    expect(onSave).not.toHaveBeenCalled();
  });

  it('flags the duplicate field name in UpdateSingle mode (save guard is Setting-only)', () => {
    const addUpdateValue = jest.fn();
    const onSave = jest.fn();
    const props = makeProps({
      type: MetadataType.UpdateSingle,
      existsKeys: ['author'],
      addUpdateValue,
      onSave,
    });

    const { result } = renderHook(() => useManageValues(props));

    act(() => {
      result.current.handleChange('field', 'author');
      result.current.handleValueChange(0, 'Alice', false);
    });

    // The warning is surfaced to the user in every mode...
    expect(result.current.valueError.field).not.toBe('');

    // ...but only Setting mode hard-blocks save on it; other modes proceed.
    act(() => {
      result.current.handleSave();
    });

    expect(addUpdateValue).toHaveBeenCalled();
  });

  it('flags the duplicate field name in Manage mode (save guard is Setting-only)', () => {
    const addUpdateValue = jest.fn();
    const props = makeProps({
      type: MetadataType.Manage,
      existsKeys: ['tag'],
      addUpdateValue,
    });

    const { result } = renderHook(() => useManageValues(props));

    act(() => {
      result.current.handleChange('field', 'tag');
    });

    expect(result.current.valueError.field).not.toBe('');

    act(() => {
      result.current.handleSave();
    });

    // Manage mode does not gate save on the field error, so the queue still runs.
    expect(addUpdateValue).toHaveBeenCalled();
  });

  it('clears the field error once the user picks a non-conflicting name', () => {
    const props = makeProps({
      type: MetadataType.UpdateSingle,
      existsKeys: ['author'],
    });

    const { result } = renderHook(() => useManageValues(props));

    act(() => {
      result.current.handleChange('field', 'author');
    });
    expect(result.current.valueError.field).not.toBe('');

    act(() => {
      result.current.handleChange('field', 'reviewer');
    });
    expect(result.current.valueError.field).toBe('');
  });
});

describe('useManageValues - edit existing metadata', () => {
  it('queues per-row updates on blur and does not call addUpdateValue on save', () => {
    const addUpdateValue = jest.fn();
    const props = makeProps({
      isAddValueMode: false,
      addUpdateValue,
      data: {
        field: 'author',
        description: '',
        values: ['Alice', 'Bob'],
        valueType: metadataValueTypeEnum.string,
      },
    });

    const { result } = renderHook(() => useManageValues(props));

    act(() => {
      result.current.handleValueChange(0, 'Alicia', false);
    });
    act(() => {
      result.current.handleValueBlur();
    });

    expect(addUpdateValue).toHaveBeenCalledTimes(1);
    expect(addUpdateValue.mock.calls[0][0]).toBe('author');
    expect(addUpdateValue.mock.calls[0][1]).toBe('Alice');
    expect(addUpdateValue.mock.calls[0][2]).toBe('Alicia');

    act(() => {
      result.current.handleSave();
    });

    expect(addUpdateValue).toHaveBeenCalledTimes(1);
  });

  it('treats indices beyond the original list as new additions on blur', () => {
    const addUpdateValue = jest.fn();
    const props = makeProps({
      isAddValueMode: false,
      addUpdateValue,
      data: {
        field: 'author',
        description: '',
        values: ['Alice'],
        valueType: metadataValueTypeEnum.string,
      },
    });

    const { result } = renderHook(() => useManageValues(props));

    act(() => {
      result.current.handleAddValue();
    });
    act(() => {
      result.current.handleValueChange(1, 'Carol', false);
    });
    act(() => {
      result.current.handleValueBlur();
    });

    const newAdditionCall = addUpdateValue.mock.calls.find(
      (call) => call[1] === '',
    );
    expect(newAdditionCall).toBeDefined();
    expect(newAdditionCall?.[2]).toBe('Carol');
  });

  it('handleValueChange with isUpdate=true syncs to addUpdateValue immediately', () => {
    const addUpdateValue = jest.fn();
    const props = makeProps({
      isAddValueMode: false,
      addUpdateValue,
      data: {
        field: 'author',
        description: '',
        values: ['Alice'],
        valueType: metadataValueTypeEnum.string,
      },
    });

    const { result } = renderHook(() => useManageValues(props));

    act(() => {
      result.current.handleValueChange(0, 'Alicia', true);
    });

    expect(addUpdateValue).toHaveBeenCalled();
    expect(addUpdateValue.mock.calls[0][1]).toBe('Alice');
    expect(addUpdateValue.mock.calls[0][2]).toBe('Alicia');
  });
});

describe('useManageValues - duplicate value guard', () => {
  it('flags an error when a typed value collides with an existing temp value', () => {
    const props = makeProps({
      data: {
        field: 'author',
        description: '',
        values: ['Alice', ''],
        valueType: metadataValueTypeEnum.string,
      },
    });

    const { result } = renderHook(() => useManageValues(props));

    act(() => {
      result.current.handleValueChange(1, 'Alice', false);
    });

    expect(result.current.valueError.values).not.toBe('');
  });

  it('clears the value error once the duplicate is replaced with a unique value', () => {
    const props = makeProps({
      data: {
        field: 'author',
        description: '',
        values: ['Alice', ''],
        valueType: metadataValueTypeEnum.string,
      },
    });

    const { result } = renderHook(() => useManageValues(props));

    act(() => {
      result.current.handleValueChange(1, 'Alice', false);
    });
    expect(result.current.valueError.values).not.toBe('');

    act(() => {
      result.current.handleValueChange(1, 'Bob', false);
    });
    expect(result.current.valueError.values).toBe('');
  });
});

describe('useManageValues - delete and clear', () => {
  it('removes the value at the given index from both tempValues and metaData', () => {
    const addDeleteValue = jest.fn();
    const props = makeProps({
      isAddValueMode: false,
      addDeleteValue,
      data: {
        field: 'author',
        description: '',
        values: ['Alice', 'Bob', 'Carol'],
        valueType: metadataValueTypeEnum.string,
      },
    });

    const { result } = renderHook(() => useManageValues(props));

    act(() => {
      result.current.handleDelete(1);
    });

    expect(result.current.tempValues).toEqual(['Alice', 'Carol']);
    expect(result.current.metaData.values).toEqual(['Alice', 'Carol']);
    expect(addDeleteValue).toHaveBeenCalledWith('author', 'Bob');
  });

  it('handleClearValues default resets to a single empty input', () => {
    const props = makeProps({
      data: {
        field: 'author',
        description: '',
        values: ['Alice', 'Bob'],
        valueType: metadataValueTypeEnum.string,
      },
    });

    const { result } = renderHook(() => useManageValues(props));

    act(() => {
      result.current.handleClearValues();
    });

    expect(result.current.tempValues).toEqual(['']);
    expect(result.current.metaData.values).toEqual(['']);
  });

  it('handleClearValues with isClearInitialValues=true empties the values entirely', () => {
    const props = makeProps({
      data: {
        field: 'author',
        description: '',
        values: ['Alice', 'Bob'],
        valueType: metadataValueTypeEnum.string,
      },
    });

    const { result } = renderHook(() => useManageValues(props));

    act(() => {
      result.current.handleClearValues(true);
    });

    expect(result.current.tempValues).toEqual([]);
    expect(result.current.metaData.values).toEqual([]);
  });
});

describe('useManageValues - valueType change', () => {
  it('preserves existing values when switching valueType', () => {
    const props = makeProps({
      data: {
        field: 'author',
        description: '',
        values: ['Alice'],
        valueType: metadataValueTypeEnum.string,
      },
    });

    const { result } = renderHook(() => useManageValues(props));

    act(() => {
      result.current.handleChange('valueType', 'list');
    });

    expect(result.current.metaData.valueType).toBe('list');
    expect(result.current.metaData.values).toEqual(['Alice']);
  });

  it('falls back to string when valueType is set to an empty value', () => {
    const props = makeProps();

    const { result } = renderHook(() => useManageValues(props));

    act(() => {
      result.current.handleChange('valueType', undefined);
    });

    expect(result.current.metaData.valueType).toBe('string');
  });

  it('passes the chosen valueType through to addUpdateValue on save', () => {
    const addUpdateValue = jest.fn();
    const props = makeProps({ addUpdateValue });

    const { result } = renderHook(() => useManageValues(props));

    act(() => {
      result.current.handleChange('field', 'tags');
      result.current.handleChange('valueType', 'list');
      result.current.handleValueChange(0, 'tag-a', false);
    });

    act(() => {
      result.current.handleSave();
    });

    expect(addUpdateValue).toHaveBeenCalledTimes(1);
    const [, , , passedType] = addUpdateValue.mock.calls[0];
    expect(passedType).toBe('list');
  });
});

describe('useManageValues - add value row', () => {
  it('handleAddValue appends an empty slot to tempValues and metaData', () => {
    const props = makeProps({
      data: {
        field: 'author',
        description: '',
        values: ['Alice'],
        valueType: metadataValueTypeEnum.string,
      },
    });

    const { result } = renderHook(() => useManageValues(props));

    act(() => {
      result.current.handleAddValue();
    });

    expect(result.current.tempValues).toEqual(['Alice', '']);
    expect(result.current.metaData.values).toEqual(['Alice', '']);
  });

  it('deduplicates when handleAddValue would repeat an empty slot', () => {
    const props = makeProps();

    const { result } = renderHook(() => useManageValues(props));

    act(() => {
      result.current.handleAddValue();
    });
    act(() => {
      result.current.handleAddValue();
    });

    expect(result.current.tempValues).toEqual(['']);
  });
});
