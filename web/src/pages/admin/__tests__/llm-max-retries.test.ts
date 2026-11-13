import { parseLlmMaxRetriesInput } from '../llm-max-retries';

describe('parseLlmMaxRetriesInput', () => {
  it.each([
    ['', { valid: true, value: null }],
    ['   ', { valid: true, value: null }],
    ['0', { valid: true, value: 0 }],
    ['3', { valid: true, value: 3 }],
    [' 7 ', { valid: true, value: 7 }],
    ['10', { valid: true, value: 10 }],
    ['010', { valid: true, value: 10 }],
    ['11', { valid: false, reason: 'outOfRange' }],
    ['-1', { valid: false, reason: 'outOfRange' }],
    ['999999999999', { valid: false, reason: 'outOfRange' }],
    ['2.5', { valid: false, reason: 'notInteger' }],
    ['1e1', { valid: false, reason: 'notInteger' }],
    ['abc', { valid: false, reason: 'notInteger' }],
    ['3 4', { valid: false, reason: 'notInteger' }],
  ])('parses %j', (input, expected) => {
    expect(parseLlmMaxRetriesInput(input)).toEqual(expected);
  });
});
