import { useEffect, useMemo } from 'react';
import { ControllerRenderProps, useFormContext } from 'react-hook-form';

import { Checkbox } from '@/components/ui/checkbox';
import { Input } from '@/components/ui/input';
import { cn } from '@/lib/utils';

/* ---------------- Token Field ---------------- */

export type ConfluenceTokenFieldProps = ControllerRenderProps & {
  fieldType: 'username' | 'token';
  placeholder?: string;
  disabled?: boolean;
};

const ConfluenceTokenField = ({
  fieldType,
  value,
  onChange,
  placeholder,
  disabled,
  ...rest
}: ConfluenceTokenFieldProps) => {
  return (
    <div className="flex w-full flex-col gap-2">
      <Input
        className="w-full"
        type={fieldType === 'token' ? 'password' : 'text'}
        value={value ?? ''}
        onChange={(e) => onChange(e.target.value)}
        placeholder={
          placeholder ||
          (fieldType === 'token'
            ? 'Enter your Confluence access token'
            : 'Confluence username or email')
        }
        disabled={disabled}
        {...rest}
      />
    </div>
  );
};

/* ---------------- Indexing Mode Field ---------------- */

type ConfluenceIndexingMode = 'everything' | 'space' | 'page';

export type ConfluenceIndexingModeFieldProps = ControllerRenderProps;

export const ConfluenceIndexingModeField = (
  fieldProps: ConfluenceIndexingModeFieldProps,
) => {
  const { value, onChange, disabled } = fieldProps;
  const { watch, setValue } = useFormContext();

  const mode = useMemo<ConfluenceIndexingMode>(
    () => (value as ConfluenceIndexingMode) || 'everything',
    [value],
  );

  const spaceValue = watch('config.space');
  const pageIdValue = watch('config.page_id');
  const indexRecursively = watch('config.index_recursively');

  useEffect(() => {
    if (!value) onChange('everything');
  }, [value, onChange]);

  const handleModeChange = (nextMode?: string) => {
    const normalized = (nextMode || 'everything') as ConfluenceIndexingMode;
    onChange(normalized);

    if (normalized === 'everything') {
      setValue('config.space', '', { shouldDirty: true, shouldTouch: true });
      setValue('config.page_id', '', { shouldDirty: true, shouldTouch: true });
      setValue('config.index_recursively', false, {
        shouldDirty: true,
        shouldTouch: true,
      });
    } else if (normalized === 'space') {
      setValue('config.page_id', '', { shouldDirty: true, shouldTouch: true });
      setValue('config.index_recursively', false, {
        shouldDirty: true,
        shouldTouch: true,
      });
    } else if (normalized === 'page') {
      setValue('config.space', '', { shouldDirty: true, shouldTouch: true });
    }
  };

  return (
    <div className="w-full rounded-lg border border-border-button bg-bg-card p-4 space-y-4">
      <div className="flex items-center gap-2 text-sm font-medium text-text-secondary">
        {INDEX_MODE_OPTIONS.map((option) => {
          const isActive = option.value === mode;
          return (
            <button
              key={option.value}
              type="button"
              disabled={disabled}
              onClick={() => handleModeChange(option.value)}
              className={cn(
                'flex-1 rounded-lg border px-3 py-2 transition-all',
                'border-transparent bg-transparent text-text-secondary hover:border-border-button hover:bg-bg-card-secondary',
                isActive &&
                  'border-border-button bg-background text-primary shadow-sm',
              )}
            >
              {option.label}
            </button>
          );
        })}
      </div>

      {mode === 'everything' && (
        <p className="text-sm text-text-secondary">
          This connector will index all pages the provided credentials have
          access to.
        </p>
      )}

      {mode === 'space' && (
        <div className="space-y-2">
          <div className="text-sm font-semibold text-text-primary">
            Space Key
          </div>
          <Input
            className="w-full"
            value={spaceValue ?? ''}
            onChange={(e) =>
              setValue('config.space', e.target.value, {
                shouldDirty: true,
                shouldTouch: true,
              })
            }
            placeholder="e.g. KB"
            disabled={disabled}
          />
          <p className="text-xs text-text-secondary">
            The Confluence space key to index.
          </p>
        </div>
      )}

      {mode === 'page' && (
        <div className="space-y-2">
          <div className="text-sm font-semibold text-text-primary">Page ID</div>
          <Input
            className="w-full"
            value={pageIdValue ?? ''}
            onChange={(e) =>
              setValue('config.page_id', e.target.value, {
                shouldDirty: true,
                shouldTouch: true,
              })
            }
            placeholder="e.g. 123456"
            disabled={disabled}
          />
          <p className="text-xs text-text-secondary">
            The Confluence page ID to index.
          </p>

          <div className="flex items-center gap-2 pt-2">
            <Checkbox
              checked={Boolean(indexRecursively)}
              onCheckedChange={(checked) =>
                setValue('config.index_recursively', Boolean(checked), {
                  shouldDirty: true,
                  shouldTouch: true,
                })
              }
              disabled={disabled}
            />
            <span className="text-sm text-text-secondary">
              Index child pages recursively
            </span>
          </div>
        </div>
      )}
    </div>
  );
};

const INDEX_MODE_OPTIONS = [
  { label: 'Everything', value: 'everything' },
  { label: 'Space', value: 'space' },
  { label: 'Page', value: 'page' },
];

export default ConfluenceTokenField;
