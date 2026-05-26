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
