import { fireEvent, render, screen } from '@testing-library/react';
import RetryableError from './retryable-error';

it('shows the message with a Retry button that calls onRetry', () => {
  const onRetry = jest.fn();
  render(<RetryableError message="Model unavailable" onRetry={onRetry} />);

  expect(screen.getByRole('alert')).toHaveTextContent('Model unavailable');
  fireEvent.click(screen.getByRole('button', { name: 'common.retry' }));
  expect(onRetry).toHaveBeenCalledTimes(1);
});

it('falls back to a generic message', () => {
  render(<RetryableError onRetry={jest.fn()} />);
  expect(screen.getByRole('alert')).toHaveTextContent('message.requestError');
});

it('disables Retry while the retried request is running', () => {
  const onRetry = jest.fn();
  render(<RetryableError message="x" onRetry={onRetry} retrying />);

  const button = screen.getByRole('button', { name: 'common.retry' });
  expect(button).toBeDisabled();
  fireEvent.click(button);
  expect(onRetry).not.toHaveBeenCalled();
});
