import { fireEvent, render, screen } from '@testing-library/react';
import { ModelVerifyButton } from './model-verify-button';

const Translations: Record<string, string> = {
  'common.retry': 'Thử lại',
  'setting.Verify': 'Xác minh',
};

jest.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => Translations[key] ?? key }),
}));

it.each([
  ['idle', 'Xác minh glm-4'],
  ['success', 'Xác minh glm-4'],
  ['error', 'Thử lại glm-4'],
] as const)('labels the %s state in the active language', (status, label) => {
  render(
    <ModelVerifyButton
      status={status}
      onVerify={jest.fn()}
      modelName="glm-4"
    />,
  );
  expect(screen.getByRole('button', { name: label })).toBeInTheDocument();
});

it('shows the translated Retry as a tooltip only after an error', () => {
  const { rerender } = render(
    <ModelVerifyButton status="error" onVerify={jest.fn()} modelName="glm-4" />,
  );
  expect(screen.getByTitle('Thử lại')).toBeInTheDocument();

  rerender(
    <ModelVerifyButton status="idle" onVerify={jest.fn()} modelName="glm-4" />,
  );
  expect(screen.queryByTitle('Thử lại')).not.toBeInTheDocument();
});

it('re-runs the verification when clicked after an error', () => {
  const onVerify = jest.fn();
  render(
    <ModelVerifyButton status="error" onVerify={onVerify} modelName="glm-4" />,
  );

  fireEvent.click(screen.getByRole('button', { name: 'Thử lại glm-4' }));
  expect(onVerify).toHaveBeenCalledTimes(1);
});
