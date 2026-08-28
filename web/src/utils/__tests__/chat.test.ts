import {
  buildPrologueMessage,
  dropPrologueFromMessages,
  mergePrologueIntoMessages,
  preprocessLaTeX,
  PROLOGUE_MESSAGE_ID,
  replaceThinkToSection,
} from '../chat';
import { MessageType } from '@/constants/chat';
import { IMessage } from '@/interfaces/database/chat';

describe('preprocessLaTeX', () => {
  it('converts block \\[ \\] to $$ $$', () => {
    expect(preprocessLaTeX('\\[ x + y \\]')).toBe('$$x + y$$');
  });

  it('converts inline \\( \\) to $ $', () => {
    expect(preprocessLaTeX('\\( a \\)')).toBe('$a$');
  });

  it('does not cut block math at \\right] (Closes #13134)', () => {
    const content =
      '\\[ C_{seq}(y|x) = \\frac{1}{|y|} \\sum_{t=1}^{|y|} \\right] \\]';
    const result = preprocessLaTeX(content);
    expect(result).toContain('\\right]');
    expect(result).toContain('\\frac{1}{|y|}');
    expect(result).toBe(
      '$$ C_{seq}(y|x) = \\frac{1}{|y|} \\sum_{t=1}^{|y|} \\right] $$',
    );
  });

  it('does not cut inline math at \\big) or nested parens', () => {
    const content = '\\( f(x) + \\big) \\)';
    const result = preprocessLaTeX(content);
    expect(result).toContain('\\big)');
    expect(result).toBe('$ f(x) + \\big) $');
  });

  it('handles multiple block equations', () => {
    const content = 'First \\[ a \\] then \\[ b \\right] c \\]';
    const result = preprocessLaTeX(content);
    expect(result).toBe('First $$a$$ then $$ b \\right] c $$');
  });
});

describe('replaceThinkToSection', () => {
  it('drops an empty think section instead of rendering a bare strip', () => {
    expect(replaceThinkToSection('<think></think>Here is the answer.')).toBe(
      'Here is the answer.',
    );
  });

  it('drops a whitespace-only think section', () => {
    expect(replaceThinkToSection('<think>  \n </think>answer')).toBe('answer');
  });

  it('keeps a non-empty think section as a details block', () => {
    expect(replaceThinkToSection('<think>some reasoning</think>answer')).toBe(
      '<details class="think"><summary>Thinking...</summary>some reasoning</details>answer',
    );
  });

  it('uses the provided summary for non-empty sections', () => {
    expect(
      replaceThinkToSection('<think>reasoning</think>', 'Deep thought'),
    ).toBe(
      '<details class="think"><summary>Deep thought</summary>reasoning</details>',
    );
  });

  it('leaves text without think markers unchanged', () => {
    expect(replaceThinkToSection('plain answer')).toBe('plain answer');
  });
});

describe('prologue message helpers', () => {
  const question = {
    id: 'question-1',
    role: MessageType.User,
    content: '你好吗',
  } as IMessage;
  const answer = {
    id: 'answer-1',
    role: MessageType.Assistant,
    content: 'I am fine.',
  } as IMessage;

  it('makes the prologue the opening message of an empty list', () => {
    expect(mergePrologueIntoMessages([], 'Hi')).toEqual([
      {
        id: PROLOGUE_MESSAGE_ID,
        role: MessageType.Assistant,
        content: 'Hi',
      },
    ]);
  });

  it('inserts the prologue above an existing conversation instead of overwriting the first message', () => {
    const result = mergePrologueIntoMessages(
      [question, answer],
      '开场白文案。',
    );
    expect(result).toHaveLength(3);
    expect(result[0]).toMatchObject({
      id: PROLOGUE_MESSAGE_ID,
      role: MessageType.Assistant,
      content: '开场白文案。',
    });
    // The user's first question must survive (it used to be overwritten).
    expect(result[1]).toEqual(question);
    expect(result[2]).toEqual(answer);
  });

  it('updates an already rendered prologue in place when the text changes', () => {
    const withPrologue = mergePrologueIntoMessages([question], 'Hi');
    const updated = mergePrologueIntoMessages(withPrologue, 'Hello');
    expect(updated).toHaveLength(2);
    expect(updated[0]).toMatchObject({
      id: PROLOGUE_MESSAGE_ID,
      content: 'Hello',
    });
    expect(updated[1]).toEqual(question);
  });

  it('returns the same array when the rendered prologue is already up to date', () => {
    const withPrologue = mergePrologueIntoMessages([question], 'Hi');
    expect(mergePrologueIntoMessages(withPrologue, 'Hi')).toBe(withPrologue);
  });

  it('drops only a leading prologue message', () => {
    const withPrologue = mergePrologueIntoMessages([question, answer], 'Hi');
    expect(dropPrologueFromMessages(withPrologue)).toEqual([question, answer]);
  });

  it('keeps the list unchanged when there is no leading prologue', () => {
    const messages = [question, answer];
    expect(dropPrologueFromMessages(messages)).toBe(messages);
    expect(dropPrologueFromMessages([])).toEqual([]);
  });

  it('never drops a prologue that is not the first message', () => {
    const messages = [question, buildPrologueMessage('Hi')] as IMessage[];
    expect(dropPrologueFromMessages(messages)).toBe(messages);
  });
});
