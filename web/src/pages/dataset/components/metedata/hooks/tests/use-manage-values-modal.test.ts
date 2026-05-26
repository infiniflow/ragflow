/**
 * Regression test for #15139.
 *
 * Reproduces the broken state-binding in the "+ Add metadata" flow:
 *   - The Input element's onChange writes to `tempValues` per keystroke but
 *     does not update `metaData.values`.
 *   - `metaData.values` only updates on the input's onBlur.
 *   - The Confirm-button click handler races the blur. The handler closes
 *     over the previous render's `metaData.values` (still the initial ['']).
 *   - Before the fix, `handleSave` passed `metaData.values` to
 *     `addUpdateValue`, so the queued update carried an empty string and the
 *     backend stored nothing — the document's metadata count stayed at "0".
 *
 * After the fix, `handleSave` reads from `tempValues` (which is synchronously
 * updated by `handleValueChange` on each keystroke), so the typed value
 * reaches the API.
 */

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

describe('useManageValues - add new metadata (regression #15139)', () => {
  it('queues the typed value, not the pre-blur empty string', () => {
    const addUpdateValue = jest.fn();
    const props = makeProps({ addUpdateValue });

    const { result } = renderHook(() => useManageValues(props));

    // Simulate the user filling the form. The field input is wired through
    // DynamicForm and synchronizes metaData.field per keystroke via
    // handleChange; the value input is a plain <Input> that only updates
    // tempValues per keystroke (handleValueBlur would later push to
    // metaData.values, but a fast click on Confirm beats the blur).
    act(() => {
      result.current.handleChange('field', 'user_name');
      result.current.handleValueChange(0, 'Employee', false);
    });

    // metaData.values is still the pre-blur initial state — exactly the
    // racy condition #15139 reports.
    expect(result.current.metaData.values).toEqual(['']);
    expect(result.current.tempValues).toEqual(['Employee']);

    act(() => {
      result.current.handleSave();
    });

    expect(addUpdateValue).toHaveBeenCalledTimes(1);
    const [, , queuedValues] = addUpdateValue.mock.calls[0];
    // Before the fix: queuedValues === [''], which the non-list branch in
    // addUpdateValue would coerce to value: '', wiping the metadata save.
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
