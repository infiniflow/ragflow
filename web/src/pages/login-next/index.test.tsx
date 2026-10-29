import { useSystemConfig } from '@/hooks/use-system-request';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import Login from './index';

jest.mock('@/hooks/use-system-request', () => ({
  useSystemConfig: jest.fn(),
}));

jest.mock('@/hooks/auth-hooks', () => ({
  useAuth: () => ({ isLogin: false }),
}));

jest.mock('@/hooks/use-login-request', () => ({
  useLogin: () => ({ login: jest.fn(), loading: false }),
  useRegister: () => ({ register: jest.fn(), loading: false }),
  useLoginChannels: () => ({ channels: [], loading: false }),
  useLoginWithChannel: () => ({ login: jest.fn(), loading: false }),
}));

jest.mock('react-router', () => ({
  useNavigate: () => jest.fn(),
}));

jest.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

jest.mock('@/utils', () => ({ rsaPsw: jest.fn() }));
jest.mock('@/components/svg-icon', () => () => null);
jest.mock('@/components/spotlight', () => () => null);
jest.mock('./bg', () => ({ BgSvg: () => null }));
jest.mock('./card', () => ({
  __esModule: true,
  default: ({ children }: { children: React.ReactNode }) => children,
  FlipFaceContext: jest.requireActual('react').createContext('front'),
}));

const MockUseSystemConfig = jest.mocked(useSystemConfig);
const OriginalResizeObserver = globalThis.ResizeObserver;

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

describe('login registration entry', () => {
  beforeEach(() => {
    MockUseSystemConfig.mockReturnValue({ config: undefined, loading: true });
  });

  it.each([0, false])(
    'keeps sign up hidden while loading and when registerEnabled is %s',
    (registerEnabled) => {
      const { rerender } = render(<Login />);

      expect(
        screen.queryByTestId('auth-toggle-register'),
      ).not.toBeInTheDocument();
      expect(screen.getByTestId('auth-submit')).toBeEnabled();

      MockUseSystemConfig.mockReturnValue({
        config: { registerEnabled },
        loading: false,
      });
      rerender(<Login />);

      expect(
        screen.queryByTestId('auth-toggle-register'),
      ).not.toBeInTheDocument();
      expect(screen.getByTestId('auth-submit')).toBeEnabled();
    },
  );

  it.each([1, true])(
    'allows switching to sign up when registerEnabled is %s',
    async (registerEnabled) => {
      const { rerender } = render(<Login />);

      expect(
        screen.queryByTestId('auth-toggle-register'),
      ).not.toBeInTheDocument();

      MockUseSystemConfig.mockReturnValue({
        config: { registerEnabled },
        loading: false,
      });
      rerender(<Login />);

      fireEvent.click(screen.getByTestId('auth-toggle-register'));

      await waitFor(() => {
        expect(screen.getByTestId('auth-nickname')).toBeInTheDocument();
      });
    },
  );

  it('keeps sign up hidden when loading ends without a configuration', () => {
    MockUseSystemConfig.mockReturnValue({ config: undefined, loading: false });

    render(<Login />);

    expect(
      screen.queryByTestId('auth-toggle-register'),
    ).not.toBeInTheDocument();
    expect(screen.getByTestId('auth-submit')).toBeEnabled();
  });
});
