import { getCachedLlmList } from '../llm-cache';
describe('model cache envelope', () => {
  beforeEach(() => {
    localStorage.clear();
    jest.useFakeTimers();
    jest.setSystemTime(new Date('2026-10-10T12:00:00Z'));
  });
  afterEach(() => {
    localStorage.clear();
    jest.useRealTimers();
  });
  test.each([
    null,
    'oops',
    { data: {} },
    { timestamp: 'invalid', data: {} },
    { timestamp: Date.now(), data: [] },
    { timestamp: Date.now(), data: 'oops' },
  ])('rejects malformed cache %p', (cache) => {
    localStorage.setItem('ragflow_llm_list_cache', JSON.stringify(cache));
    expect(getCachedLlmList()).toBeNull();
    expect(localStorage.getItem('ragflow_llm_list_cache')).toBeNull();
  });
  test('returns a valid fresh model list', () => {
    const data = { OpenAI: { llm: [{ name: 'example' }] } };
    localStorage.setItem(
      'ragflow_llm_list_cache',
      JSON.stringify({ timestamp: Date.now(), data }),
    );
    expect(getCachedLlmList()).toEqual(data);
  });
  test('removes expired model lists', () => {
    localStorage.setItem(
      'ragflow_llm_list_cache',
      JSON.stringify({ timestamp: Date.now() - 300001, data: {} }),
    );
    expect(getCachedLlmList()).toBeNull();
    expect(localStorage.getItem('ragflow_llm_list_cache')).toBeNull();
  });
});
