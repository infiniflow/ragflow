import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import VerifyButton from './verify-button';

jest.mock('@/hooks/common-hooks', () => ({
  useTranslate: () => {
    const t = (key: string) => key;
    return { t, i18n: { t, language: 'en', resolvedLanguage: 'en' } };
  },
}));
jest.mock('@/pages/dataset/process-log-modal', () => ({
  replaceText: (text: string) => text,
}));

const formRef = {
  current: { trigger: async () => true, getValues: () => ({}) },
};

it('turns into Retry after a failed verification and verifies again on click', async () => {
  const onVerify = jest
    .fn()
    .mockResolvedValueOnce({ isValid: false, logs: 'provider unreachable' })
    .mockResolvedValueOnce({ isValid: true, logs: '' });
  render(<VerifyButton onVerify={onVerify} formRef={formRef} />);

  fireEvent.click(screen.getByRole('button', { name: 'Verify' }));
  expect(await screen.findByText('provider unreachable')).toBeInTheDocument();
  const retry = screen.getByRole('button', { name: 'common.retry' });

  fireEvent.click(retry);
  await waitFor(() => expect(onVerify).toHaveBeenCalledTimes(2));
  expect(
    await screen.findByRole('button', { name: 'Verify' }),
  ).toBeInTheDocument();
});
