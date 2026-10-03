import { shouldSyncPromptEditorValue } from './utils';

describe('shouldSyncPromptEditorValue', () => {
  it('syncs an empty controlled value after non-empty content', () => {
    expect(shouldSyncPromptEditorValue('', 'existing prompt')).toBe(true);
  });

  it('does not treat an uncontrolled value as a clear request', () => {
    expect(shouldSyncPromptEditorValue(undefined, 'existing prompt')).toBe(
      false,
    );
  });

  it('does not resync an unchanged value', () => {
    expect(
      shouldSyncPromptEditorValue('existing prompt', 'existing prompt'),
    ).toBe(false);
  });
});
