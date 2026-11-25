import { useSystemConfig } from '@/hooks/use-system-request';
import { changeLanguageAsync } from '@/locales/config';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import Login from './index';

jest.mock('@/locales/config', () => ({
  changeLanguageAsync: jest.fn(),
  supportedLanguages: [
    { code: 'en', displayName: 'English' },
    { code: 'zh-Hans', displayName: 'Chinese (Simplified)' },
  ],
}));

jest.mock('@/hooks/use-system-request', () => ({
  useSystemConfig: jest.fn(),
}));

jest.mock('@/hooks/auth-hooks', () => ({
  useAuth: () => ({ isLogin: false }),
}));

jest.mock('@/hooks/use-login-request', () => ({
  useLogin: () => ({ login: jest.fn(), loading: false }),
  useRegister: () => ({ register: jest.fn(), loading: false }),
}));

jest.mock('react-router', () => ({
  useLocation: () => ({ state: null }),
  useNavigate: () => jest.fn(),
  useSearchParams: () => [new URLSearchParams()],
}));

jest.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string) => key,
    i18n: { language: 'en' },
  }),
}));

jest.mock('@/utils', () => ({ rsaPsw: jest.fn() }));
jest.mock('@/components/spotlight', () => () => null);
jest.mock('./bg', () => ({ BgSvg: () => null }));
jest.mock('./card', () => ({
  __esModule: true,
  default: ({ children }: { children: React.ReactNode }) => children,
  FlipFaceContext: jest.requireActual('react').createContext('front'),
}));

const MockUseSystemConfig = jest.mocked(useSystemConfig);
const MockChangeLanguageAsync = jest.mocked(changeLanguageAsync);
const OriginalResizeObserver = globalThis.ResizeObserver;
const OriginalScrollIntoView = Element.prototype.scrollIntoView;

beforeAll(() => {
  Element.prototype.scrollIntoView = jest.fn();
  globalThis.ResizeObserver = jest.fn().mockImplementation(() => ({
    observe: jest.fn(),
    unobserve: jest.fn(),
    disconnect: jest.fn(),
  }));
});

afterAll(() => {
  globalThis.ResizeObserver = OriginalResizeObserver;
  Element.prototype.scrollIntoView = OriginalScrollIntoView;
});

describe('login registration entry', () => {
  const config = {
    disablePasswordLogin: false,
    registerEnabled: 1,
    emailVerificationEnabled: false,
    upload_size_limit: 0,
  };

  beforeEach(() => {
    MockChangeLanguageAsync.mockReset();
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
        config: { ...config, registerEnabled },
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
        config: { ...config, registerEnabled },
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

  it('changes the display language from the card footer', async () => {
    render(<Login />);

    const languageTrigger = screen.getByTestId('auth-language-switch');
    expect(languageTrigger).toHaveTextContent('English');

    fireEvent.click(languageTrigger);
    fireEvent.click(await screen.findByTestId('auth-language-option-zh-Hans'));

    expect(MockChangeLanguageAsync).toHaveBeenCalledWith('zh-Hans');
  });
});
