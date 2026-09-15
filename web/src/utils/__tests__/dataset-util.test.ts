import { isDatasetId } from '../dataset-util';

describe('isDatasetId', () => {
  test('accepts 32-char hex ids from both backends', () => {
    expect(isDatasetId('1b33f6e2922f11f182a6eb24e39177ff')).toBe(true);
    expect(isDatasetId('1B33F6E2922F11F182A6EB24E39177FF')).toBe(true);
  });

  test('rejects variable references mixed into dataset_ids', () => {
    expect(isDatasetId('sys.query')).toBe(false);
    expect(isDatasetId('env.vvvv')).toBe(false);
    expect(isDatasetId('begin@file')).toBe(false);
    expect(isDatasetId('Retrieval:abc@content')).toBe(false);
  });

  test('rejects other non-id values', () => {
    expect(isDatasetId('')).toBe(false);
    expect(isDatasetId('timeline002')).toBe(false);
    expect(isDatasetId('1b33f6e2-922f-11f1-82a6-eb24e39177ff')).toBe(false);
    expect(isDatasetId('1b33f6e2922f11f182a6eb24e39177f')).toBe(false);
  });
});
