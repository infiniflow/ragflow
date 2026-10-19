import { render, screen } from '@testing-library/react';
import { TooltipProvider } from '@/components/ui/tooltip';
import RewriteQuestionForm from '../index';

jest.mock('@/pages/agent/context', () => ({
  useOwnerTenantId: () => 'tenant-1',
}));

jest.mock('@/pages/agent/hooks/use-watch-form-change', () => ({
  useWatchFormChange: jest.fn(),
}));

jest.mock('@/components/model-tree-select', () => ({
  ModelTreeSelectFormField: ({ name }: any) => {
    const {
      Controller,
      useFormContext,
    }: typeof import('react-hook-form') = require('react-hook-form');
    const { control } = useFormContext();
    return (
      <Controller
        control={control}
        name={name}
        render={({ field }) => <input aria-label={name} {...field} />}
      />
    );
  },
}));

jest.mock('@/components/message-history-window-size-item', () => ({
  MessageHistoryWindowSizeFormField: () => {
    const {
      Controller,
      useFormContext,
    }: typeof import('react-hook-form') = require('react-hook-form');
    const { control } = useFormContext();
    return (
      <Controller
        control={control}
        name="message_history_window_size"
        render={({ field }) => (
          <input
            aria-label="message_history_window_size"
            type="number"
            {...field}
          />
        )}
      />
    );
  },
}));

jest.mock('@/components/originui/select-with-search', () => {
  const { forwardRef } = require('react');

  return {
    SelectWithSearch: forwardRef(
      ({ value, onChange }: any, ref: React.ForwardedRef<HTMLInputElement>) => (
        <input
          ref={ref}
          aria-label="language"
          value={value ?? ''}
          onChange={onChange}
        />
      ),
    ),
  };
});

jest.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

describe('RewriteQuestionForm', () => {
  it('renders with the node passed by FormSheet and initializes from its form', () => {
    const node = {
      id: 'rewrite:0',
      position: { x: 0, y: 0 },
      data: {
        label: 'RewriteQuestion',
        name: 'rewrite:0',
        form: {
          llm_id: 'model-1',
          language: 'English',
          message_history_window_size: 4,
        },
      },
    };

    render(
      <TooltipProvider>
        <RewriteQuestionForm node={node as any} />
      </TooltipProvider>,
    );

    expect(screen.getByLabelText('llm_id')).toHaveValue('model-1');
    expect(screen.getByLabelText('language')).toHaveValue('English');
    expect(screen.getByLabelText('message_history_window_size')).toHaveValue(4);
  });
});
