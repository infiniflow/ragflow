import { fireEvent, render, screen } from '@testing-library/react';
import {
  SAVED_CREDENTIAL_MASK,
  SavedCredentialInput,
} from './saved-credential-input';

describe('SavedCredentialInput', () => {
  it('keeps the saved credential mask selected on mouse release', () => {
    render(
      <SavedCredentialInput
        aria-label="API key"
        value={SAVED_CREDENTIAL_MASK}
        maskedValue="sk-a**********z9"
      />,
    );

    const input = screen.getByLabelText('API key') as HTMLInputElement;
    const mouseUp = new MouseEvent('mouseup', {
      bubbles: true,
      cancelable: true,
    });
    fireEvent(input, mouseUp);

    expect(mouseUp.defaultPrevented).toBe(true);
    expect(input.selectionStart).toBe(0);
    expect(input.selectionEnd).toBe(input.value.length);
  });
});
