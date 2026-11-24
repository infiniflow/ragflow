import {
  fireEvent,
  render,
  renderHook,
  screen,
  waitFor,
} from '@testing-library/react';

import useCreateUserForm, { CreateUserForm } from './user-form';

jest.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

it('submits the nickname as the display name', async () => {
  const { result } = renderHook(() => useCreateUserForm());
  const onSubmit = jest.fn();
  render(
    <CreateUserForm
      id="create-user"
      form={result.current.form}
      onSubmit={onSubmit}
    />,
  );

  fireEvent.change(screen.getByLabelText('admin.email'), {
    target: { value: 'alice@example.com' },
  });
  fireEvent.change(screen.getByLabelText('admin.nickname'), {
    target: { value: '  Alice 张 😀  ' },
  });
  fireEvent.change(screen.getByLabelText('admin.password'), {
    target: { value: 'secret123' },
  });
  fireEvent.change(screen.getByLabelText('admin.confirmPassword'), {
    target: { value: 'secret123' },
  });
  fireEvent.submit(document.getElementById('create-user')!);

  await waitFor(() => expect(onSubmit).toHaveBeenCalled());
  expect(onSubmit.mock.calls[0][0]).toEqual({
    email: 'alice@example.com',
    nickname: 'Alice 张 😀',
    password: 'secret123',
    confirmPassword: 'secret123',
  });
});

it('allows an empty nickname and validates the Unicode character limit', () => {
  const { result } = renderHook(() => useCreateUserForm());
  const values = {
    email: 'alice@example.com',
    password: 'secret123',
    confirmPassword: 'secret123',
  };

  expect(result.current.form.getValues('nickname')).toBe('');
  for (const nickname of ['', ' \t ', '😀'.repeat(100)]) {
    expect(
      result.current.schema.safeParse({ ...values, nickname }).success,
    ).toBe(true);
  }
  const invalid = result.current.schema.safeParse({
    ...values,
    nickname: '😀'.repeat(101),
  });
  expect(invalid.success).toBe(false);
  if (!invalid.success) {
    expect(invalid.error.issues[0].path).toEqual(['nickname']);
    expect(invalid.error.issues[0].message).toBe('admin.nicknameMaxLength');
  }
});
