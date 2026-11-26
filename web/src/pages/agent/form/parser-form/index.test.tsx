import { render, screen } from '@testing-library/react';
import ParserForm from '.';

jest.mock('react-i18next', () => ({
  ...jest.requireActual('react-i18next'),
  useTranslation: () => ({ t: (key: string) => key }),
}));

jest.mock('@/hooks/use-llm-request', () => ({
  useFetchDefaultModelDictionary: () => ({}),
}));

jest.mock('../../hooks/use-form-values', () => ({
  useFormValues: (_defaults: unknown, node: any) => node?.data?.form ?? {},
}));

jest.mock('../../hooks/use-watch-form-change', () => ({
  useWatchFormChange: () => {},
}));

jest.mock('../../hooks/use-form-change-callback', () => ({
  useFormChangeCallback: () => {},
}));

jest.mock(
  '@/components/pipeline-operator-tabs/use-sync-external-form-errors',
  () => ({
    useSyncExternalFormErrors: () => {},
  }),
);

jest.mock('@/ee/components/parser-extension-fields', () => ({
  ParserExtensionFields: () => <span>extension fields</span>,
}));

jest.mock('./vision-form-fields', () => ({
  VisionEnhancementFormFields: () => <span>vision fields</span>,
}));

const node = {
  id: 'Parser:parse',
  data: {
    form: {
      enable_vision_enhancement: true,
      vlm: { llm_id: '', system_prompt: '', lang: '' },
      setups: [{ fileFormat: 'json' }],
    },
  },
} as any;

describe('ParserForm visionOnly', () => {
  it('renders only the global vision fields', () => {
    render(<ParserForm node={node} visionOnly />);

    expect(screen.getByText('vision fields')).toBeInTheDocument();
    expect(screen.queryByText('extension fields')).not.toBeInTheDocument();
    expect(
      screen.queryByText('flow.fileFormatOptions.json'),
    ).not.toBeInTheDocument();
  });

  it('renders the file setups when not visionOnly', () => {
    render(<ParserForm node={node} />);

    expect(screen.getByText('vision fields')).toBeInTheDocument();
    expect(screen.getByText('extension fields')).toBeInTheDocument();
    expect(screen.getByText('flow.fileFormatOptions.json')).toBeInTheDocument();
  });
});
