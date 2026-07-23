import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Input, InputProps } from '@/components/ui/input';
import { cn } from '@/lib/utils';
import { Plus, X } from 'lucide-react';
import {
  forwardRef,
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from 'react';
import { useFormContext } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from './ui/form';

interface IProps {
  value?: string | undefined;
  onChange?: (val: string | undefined) => void;
}

export const DelimiterInput = forwardRef<HTMLInputElement, InputProps & IProps>(
  function DelimiterInput(
    { value, onChange, maxLength, defaultValue, ...props },
    ref,
  ) {
    const nextValue = value
      ?.replaceAll('\n', '\\n')
      .replaceAll('\t', '\\t')
      .replaceAll('\r', '\\r');
    const handleInputChange = (e: React.ChangeEvent<HTMLInputElement>) => {
      const val = e.target.value;
      const nextValue = val
        .replaceAll('\\n', '\n')
        .replaceAll('\\t', '\t')
        .replaceAll('\\r', '\r');
      onChange?.(nextValue);
    };
    return (
      <Input
        value={nextValue}
        onChange={handleInputChange}
        maxLength={maxLength}
        defaultValue={defaultValue}
        ref={ref}
        className={cn('bg-bg-base', props.className)}
        {...props}
      ></Input>
    );
  },
);

// Parse a delimiter string into discrete blocks.
// Multi-char delimiters are wrapped in backticks per the chunker contract
// (see rag/nlp/__init__.py::naive_merge): every other char is a single-char
// delimiter on its own.
export function parseDelimiterBlocks(s: string): string[] {
  const out: string[] = [];
  let i = 0;
  while (i < s.length) {
    if (s[i] === '`') {
      const end = s.indexOf('`', i + 1);
      if (end > i) {
        const inner = s.slice(i + 1, end);
        if (inner) out.push(inner);
        i = end + 1;
        continue;
      }
    }
    out.push(s[i]);
    i++;
  }
  return out;
}

export function serializeDelimiterBlocks(blocks: string[]): string {
  return blocks
    .filter((b) => b.length > 0)
    .map((b) => (b.length > 1 ? `\`${b}\`` : b))
    .join('');
}

function describeBlock(b: string): string {
  if (b === '\n') return '↵';
  if (b === '\t') return '⇥';
  if (b === '\r') return '↩';
  return b;
}

interface ChipProps {
  block: string;
  onRemove: () => void;
}

function DelimiterChip({ block, onRemove }: ChipProps) {
  const { t } = useTranslation();
  return (
    <Badge
      variant="secondary"
      className="gap-1 px-2 py-1 font-mono text-sm cursor-default"
    >
      <span>{describeBlock(block)}</span>
      <button
        type="button"
        aria-label={t('knowledgeDetails.delimiterRemove')}
        onClick={onRemove}
        className="rounded-sm opacity-60 hover:opacity-100 hover:text-state-error outline-none focus-visible:ring-2 focus-visible:ring-state-error/40 focus-visible:ring-offset-1 focus-visible:opacity-100"
      >
        <X className="size-3" />
      </button>
    </Badge>
  );
}

interface BuilderProps {
  value?: string;
  onChange?: (val: string) => void;
  presets?: Array<{ label: string; value: string }>;
}

export function DelimiterBuilder({ value, onChange, presets }: BuilderProps) {
  const { t } = useTranslation();
  const blocks = useMemo(() => parseDelimiterBlocks(value ?? ''), [value]);
  const [adding, setAdding] = useState(false);
  const [draft, setDraft] = useState('');
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (adding) inputRef.current?.focus();
  }, [adding]);

  const commit = useCallback(
    (next: string[]) => onChange?.(serializeDelimiterBlocks(next)),
    [onChange],
  );

  const appendRaw = useCallback(
    (raw: string) => {
      if (!raw) return;
      // Wire format reserves the backtick to wrap multi-char blocks and has
      // no escape syntax (see rag/nlp/__init__.py::get_delimiters), so a
      // user-entered ` cannot round-trip — strip it at the input boundary.
      const decoded = raw
        .replaceAll('\\n', '\n')
        .replaceAll('\\t', '\t')
        .replaceAll('\\r', '\r')
        .replaceAll('`', '');
      if (!decoded) return;
      commit([...blocks, decoded]);
    },
    [blocks, commit],
  );

  const handleSubmit = useCallback(() => {
    appendRaw(draft);
    setDraft('');
    setAdding(false);
  }, [appendRaw, draft]);

  const defaultPresets = useMemo(
    () => [
      { label: `↵ ${t('knowledgeDetails.delimiterPresetNewline')}`, value: '\n' },
      { label: `⇥ ${t('knowledgeDetails.delimiterPresetTab')}`, value: '\t' },
      { label: `## ${t('knowledgeDetails.delimiterPresetHeading')}`, value: '##' },
      { label: `--- ${t('knowledgeDetails.delimiterPresetHr')}`, value: '---' },
    ],
    [t],
  );

  const effectivePresets = presets ?? defaultPresets;

  return (
    <div className="space-y-2">
      <div className="flex flex-wrap items-center gap-1.5 min-h-9 rounded-md border border-input bg-bg-base px-2 py-1.5">
        {blocks.map((b, i) => (
          <DelimiterChip
            key={`${i}-${b}`}
            block={b}
            onRemove={() => commit(blocks.filter((_, idx) => idx !== i))}
          />
        ))}
        {adding ? (
          <Input
            ref={inputRef}
            value={draft}
            onChange={(e) => setDraft(e.target.value)}
            onBlur={handleSubmit}
            onKeyDown={(e) => {
              if (e.key === 'Enter') {
                if (e.nativeEvent.isComposing || e.keyCode === 229) return;
                e.preventDefault();
                handleSubmit();
              } else if (e.key === 'Escape') {
                e.preventDefault();
                setDraft('');
                setAdding(false);
              }
            }}
            placeholder={t('knowledgeDetails.delimiterAddPlaceholder')}
            className="h-7 w-32 px-2 py-1 text-sm"
          />
        ) : (
          <Button
            type="button"
            size="sm"
            variant="ghost"
            className="h-7 px-2"
            onClick={() => setAdding(true)}
          >
            <Plus className="size-3.5" />
            {t('common.add')}
          </Button>
        )}
      </div>
      <div className="flex flex-wrap items-center gap-1.5">
        <span className="text-xs text-text-sub-title">
          {t('knowledgeDetails.delimiterPresetsLabel')}
        </span>
        {effectivePresets.map((p) => (
          <Button
            key={p.value}
            type="button"
            size="sm"
            variant="outline"
            className="h-6 px-2 font-mono text-xs"
            onClick={() => commit([...blocks, p.value])}
          >
            {p.label}
          </Button>
        ))}
      </div>
    </div>
  );
}

export function DelimiterFormField() {
  const { t } = useTranslation();
  const form = useFormContext();

  return (
    <FormField
      control={form.control}
      name={'parser_config.delimiter'}
      render={({ field }) => {
        if (typeof field.value === 'undefined') {
          // default value set
          form.setValue('parser_config.delimiter', '\n');
        }
        return (
          <FormItem className=" items-center space-y-0 ">
            <div className="flex items-start gap-1">
              <FormLabel
                required
                tooltip={t('knowledgeDetails.delimiterTip')}
                className="text-sm text-text-secondary whitespace-break-spaces w-1/4 pt-2"
              >
                {t('knowledgeDetails.delimiter')}
              </FormLabel>
              <div className="w-3/4">
                <FormControl>
                  <DelimiterBuilder
                    value={field.value}
                    onChange={field.onChange}
                  />
                </FormControl>
              </div>
            </div>
            <div className="flex pt-1">
              <div className="w-1/4"></div>
              <FormMessage />
            </div>
          </FormItem>
        );
      }}
    />
  );
}
