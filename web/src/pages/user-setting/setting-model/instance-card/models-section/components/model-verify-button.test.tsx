import { fireEvent, render, screen } from '@testing-library/react';
import { ModelVerifyButton } from './model-verify-button';

it.each([
  ['idle', 'Verify glm-4'],
  ['success', 'Verify glm-4'],
  ['error', 'Retry glm-4'],
] as const)('labels the %s state as %s', (status, label) => {
  render(
    <ModelVerifyButton
      status={status}
      onVerify={jest.fn()}
      modelName="glm-4"
    />,
  );
  expect(screen.getByRole('button', { name: label })).toBeInTheDocument();
});

it('re-runs the verification when clicked after an error', () => {
  const onVerify = jest.fn();
  render(
    <ModelVerifyButton status="error" onVerify={onVerify} modelName="glm-4" />,
  );

  fireEvent.click(screen.getByRole('button', { name: 'Retry glm-4' }));
  expect(onVerify).toHaveBeenCalledTimes(1);
});
