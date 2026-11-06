import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { AxiosHeaders } from 'axios';

import {
  AdminAuthorization,
  AdminToken,
  AdminUserInfo,
} from '@/constants/authorization';
import { login } from '@/services/admin-service';

import AdminLogin from './login';
import { CurrentUserInfoContext } from './layouts/root-layout';

const MockNavigate = jest.fn();

jest.mock('@/services/admin-service', () => ({ login: jest.fn() }));
jest.mock('@/hooks/auth-hooks', () => ({
  useAuth: () => ({ isLogin: false }),
}));
jest.mock('react-router', () => ({ useNavigate: () => MockNavigate }));
jest.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));
jest.mock('@/components/spotlight', () => () => null);
jest.mock('@/components/theme-switch', () => () => null);
jest.mock('../login-next/bg', () => ({ BgSvg: () => null }));

const MockLogin = jest.mocked(login);
const OriginalResizeObserver = globalThis.ResizeObserver;
const Email = 'admin@example.com';

function response(code: number, data: Partial<AdminService.LoginData> = {}) {
  return {
    data: { code, data: data as AdminService.LoginData, message: '' },
    headers: new AxiosHeaders({ Authorization: 'test-admin-authorization' }),
    status: 200,
    statusText: 'OK',
    config: { headers: new AxiosHeaders() },
  };
}

function renderLogin() {
  const setCurrentUserInfo = jest.fn();
  const client = new QueryClient({
    defaultOptions: { mutations: { retry: false, gcTime: Infinity } },
  });

  render(
    <QueryClientProvider client={client}>
      <CurrentUserInfoContext.Provider
        value={[{ userInfo: null, source: null }, setCurrentUserInfo]}
      >
        <AdminLogin />
      </CurrentUserInfoContext.Provider>
    </QueryClientProvider>,
  );

  return setCurrentUserInfo;
}

function submitPassword(password: string) {
  fireEvent.change(screen.getByLabelText('emailLabel'), {
    target: { value: Email },
  });
  fireEvent.change(screen.getByLabelText('passwordLabel'), {
    target: { value: password },
  });
  fireEvent.click(screen.getByRole('button', { name: 'login' }));
}

beforeAll(() => {
  globalThis.ResizeObserver = jest.fn().mockImplementation(() => ({
    observe: jest.fn(),
    unobserve: jest.fn(),
    disconnect: jest.fn(),
  }));
});

afterAll(() => {
  globalThis.ResizeObserver = OriginalResizeObserver;
});

beforeEach(() => {
  jest.clearAllMocks();
  localStorage.clear();
  MockLogin.mockResolvedValue(response(109));
});

describe('admin login password encryption', () => {
  it.each([
    ['184 ASCII characters', 'a'.repeat(184)],
    ['62 CJK characters', '密'.repeat(62)],
    ['46 emoji', '😀'.repeat(46)],
  ])('rejects %s before requesting login', async (_label, password) => {
    renderLogin();
    submitPassword(password);

    expect(await screen.findByText('passwordEncryptionError')).toBeVisible();
    expect(screen.getByLabelText('passwordLabel')).toHaveAttribute(
      'aria-invalid',
      'true',
    );
    expect(MockLogin).not.toHaveBeenCalled();
    expect(MockNavigate).not.toHaveBeenCalled();
  });

  it.each([
    ['183 ASCII characters', 'a'.repeat(183)],
    ['61 CJK characters', '密'.repeat(61)],
    ['45 emoji', '😀'.repeat(45)],
  ])('submits ciphertext for %s', async (_label, password) => {
    renderLogin();
    submitPassword(password);

    await waitFor(() => expect(MockLogin).toHaveBeenCalledTimes(1));
    const [params] = MockLogin.mock.calls[0];
    expect(params.email).toBe(Email);
    expect(typeof params.password).toBe('string');
    expect(params.password.length).toBeGreaterThan(0);
    expect(params.password).not.toBe(password);
    expect(
      screen.queryByText('passwordEncryptionError'),
    ).not.toBeInTheDocument();
  });

  it('allows correcting an oversized password and submitting again', async () => {
    renderLogin();
    submitPassword('a'.repeat(184));
    expect(await screen.findByText('passwordEncryptionError')).toBeVisible();

    submitPassword('short-password');

    await waitFor(() => expect(MockLogin).toHaveBeenCalledTimes(1));
    expect(
      screen.queryByText('passwordEncryptionError'),
    ).not.toBeInTheDocument();
    expect(MockLogin.mock.calls[0][0].password).toEqual(expect.any(String));
  });

  it('keeps an empty password in form validation', async () => {
    renderLogin();
    submitPassword('');

    expect(await screen.findByText('passwordPlaceholder')).toBeVisible();
    expect(MockLogin).not.toHaveBeenCalled();
  });

  it('keeps invalid credentials logged out', async () => {
    const setCurrentUserInfo = renderLogin();
    submitPassword('incorrect-password');

    await waitFor(() => expect(MockLogin).toHaveBeenCalledTimes(1));
    await waitFor(() =>
      expect(screen.getByRole('button', { name: 'login' })).toBeEnabled(),
    );
    expect(localStorage.getItem(AdminAuthorization)).toBeNull();
    expect(localStorage.getItem(AdminToken)).toBeNull();
    expect(localStorage.getItem(AdminUserInfo)).toBeNull();
    expect(setCurrentUserInfo).not.toHaveBeenCalled();
    expect(MockNavigate).not.toHaveBeenCalled();
  });

  it('stores successful login credentials and opens admin services', async () => {
    const data = {
      email: Email,
      nickname: 'Admin',
      access_token: 'test-admin-token',
    };
    MockLogin.mockResolvedValue(response(0, data));
    const setCurrentUserInfo = renderLogin();
    submitPassword('valid-password');

    await waitFor(() =>
      expect(MockNavigate).toHaveBeenCalledWith('/admin/services'),
    );
    expect(localStorage.getItem(AdminAuthorization)).toBe(
      'test-admin-authorization',
    );
    expect(localStorage.getItem(AdminToken)).toBe(data.access_token);
    expect(JSON.parse(localStorage.getItem(AdminUserInfo)!)).toEqual({
      ...data,
      name: data.nickname,
    });
    expect(setCurrentUserInfo).toHaveBeenCalledWith({
      userInfo: data,
      source: 'serverRequest',
    });
  });
});
