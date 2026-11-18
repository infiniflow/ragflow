import { resolveAgenticRagSubmitValue } from './agentic-rag-submit';

describe('resolveAgenticRagSubmitValue', () => {
  const DefaultContent = 'shipped default template';

  it('returns null when there is no value', () => {
    expect(resolveAgenticRagSubmitValue(undefined, DefaultContent)).toBeNull();
    expect(resolveAgenticRagSubmitValue(null, DefaultContent)).toBeNull();
  });

  it('returns null for blank content, which the backend rejects', () => {
    expect(
      resolveAgenticRagSubmitValue({ content: '   ' }, DefaultContent),
    ).toBeNull();
  });

  it('returns null when content matches the server default', () => {
    expect(
      resolveAgenticRagSubmitValue({ content: DefaultContent }, DefaultContent),
    ).toBeNull();
  });

  it('preserves a tools override when content matches the server default', () => {
    expect(
      resolveAgenticRagSubmitValue(
        { content: DefaultContent, tools: ['grep_chunks', 'list_chunks'] },
        DefaultContent,
      ),
    ).toEqual({ tools: ['grep_chunks', 'list_chunks'] });
  });

  it('keeps the content when the default is unknown (query never ran)', () => {
    expect(
      resolveAgenticRagSubmitValue({ content: DefaultContent }, undefined),
    ).toEqual({ content: DefaultContent });
  });

  it('returns custom content', () => {
    expect(
      resolveAgenticRagSubmitValue({ content: 'custom' }, DefaultContent),
    ).toEqual({ content: 'custom' });
  });

  it('preserves a tools override on the reset path', () => {
    expect(
      resolveAgenticRagSubmitValue(
        { content: '', tools: ['grep_chunks', 'list_chunks'] },
        DefaultContent,
      ),
    ).toEqual({ tools: ['grep_chunks', 'list_chunks'] });
  });

  it('preserves a tools override together with custom content', () => {
    expect(
      resolveAgenticRagSubmitValue(
        { content: 'custom', tools: ['list_chunks'] },
        DefaultContent,
      ),
    ).toEqual({ content: 'custom', tools: ['list_chunks'] });
  });
});
