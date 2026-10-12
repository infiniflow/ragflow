import { flattenVariableOptions } from './utils';

describe('flattenVariableOptions', () => {
  it('flattens groups and wraps leaf values as `{...}` reference text', () => {
    expect(
      flattenVariableOptions([
        {
          title: 'Retrieval',
          options: [
            { label: 'content', value: 'retrieval_0@content' },
            { label: 'json', value: 'retrieval_0@json' },
          ],
        },
      ]),
    ).toEqual([
      { value: '{retrieval_0@content}', label: 'Retrieval / content' },
      { value: '{retrieval_0@json}', label: 'Retrieval / json' },
    ]);
  });

  it('falls back to the bare leaf label when the group title is not a string', () => {
    expect(
      flattenVariableOptions([
        {
          title: { toString: () => 'ignored' } as never,
          options: [{ label: 'userid', value: 'begin@userid' }],
        },
      ]),
    ).toEqual([{ value: '{begin@userid}', label: 'userid' }]);
  });

  it('falls back to the raw value when the leaf label is not a string', () => {
    expect(
      flattenVariableOptions([
        {
          title: 'Begin input',
          options: [{ label: { nested: true } as never, value: 'sys.files' }],
        },
      ]),
    ).toEqual([{ value: '{sys.files}', label: 'Begin input / sys.files' }]);
  });

  it('skips groups without options and leaves without a value', () => {
    expect(
      flattenVariableOptions([
        { title: 'Empty' },
        {
          title: 'Retrieval',
          options: [{ label: 'ghost' }, { label: 'json', value: 'a@json' }],
        },
      ]),
    ).toEqual([{ value: '{a@json}', label: 'Retrieval / json' }]);
  });
});
