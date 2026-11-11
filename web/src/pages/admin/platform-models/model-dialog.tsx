import { zodResolver } from '@hookform/resolvers/zod';
import { isEqual } from 'lodash';
import { FocusEvent, useCallback, useMemo, useRef, useState } from 'react';
import { useForm } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { z } from 'zod';

import { SelectWithSearch } from '@/components/originui/select-with-search';
import { RAGFlowFormItem } from '@/components/ragflow-form';
import { Button } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Form } from '@/components/ui/form';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import message from '@/components/ui/message';
import { Switch } from '@/components/ui/switch';

import { BusyButton } from './busy-button';
import { PlatformModelTypeLabelKeys, PlatformModelTypes } from './constants';
import {
  PlatformModelKeys,
  useCloseUnlessSaving,
  useBlockBusyEvents,
  useDiscoverPlatformModels,
  useReturnFocusOnClose,
  useSavePlatformModel,
} from './hooks';

const DefaultMaxTokens = 8192;

interface PlatformModelDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  provider: AdminService.PlatformProvider;
  instance: AdminService.PlatformInstance;
  /** Absent when adding a model. */
  model?: AdminService.PlatformModel;
  existingNames: string[];
}

/** Add a platform model, or change the types and context length of one. */
export function PlatformModelDialog(props: PlatformModelDialogProps) {
  const { open, onOpenChange, model, instance } = props;
  const { t } = useTranslation();
  const returnFocus = useReturnFocusOnClose(open);
  const handleOpenChange = useCloseUnlessSaving(onOpenChange, [
    PlatformModelKeys.saveModel(props.provider.id, instance.id),
    PlatformModelKeys.discover(props.provider.id),
  ]);
  // Cancel goes through the same guard as Esc and the close button.
  const handleCancel = useCallback(
    () => handleOpenChange(false),
    [handleOpenChange],
  );
  // A fresh form on every open; while closing, the dialog keeps the form it
  // showed.
  const [formKey, setFormKey] = useState(0);
  const [wasOpen, setWasOpen] = useState(open);
  if (open !== wasOpen) {
    setWasOpen(open);
    if (open) setFormKey((key) => key + 1);
  }
  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="max-w-xl" onCloseAutoFocus={returnFocus}>
        <DialogHeader>
          <DialogTitle className="min-w-0 [overflow-wrap:anywhere]">
            {model
              ? t('admin.platformModelsPage.editModel', {
                  name: model.model_name,
                })
              : t('admin.platformModelsPage.addModelTo', {
                  name: instance.instance_name,
                })}
          </DialogTitle>
          <DialogDescription>
            {t('admin.platformModelsPage.modelDescription')}
          </DialogDescription>
        </DialogHeader>
        <ModelForm key={formKey} {...props} onCancel={handleCancel} />
      </DialogContent>
    </Dialog>
  );
}

function ModelForm({
  onOpenChange,
  onCancel,
  provider,
  instance,
  model,
  existingNames,
}: PlatformModelDialogProps & { onCancel: () => void }) {
  const { t } = useTranslation();
  const saveMutation = useSavePlatformModel(provider.id, instance.id);
  const discoverMutation = useDiscoverPlatformModels(provider.id);

  const schema = useMemo(
    () =>
      z.object({
        model_name: z
          .string()
          .trim()
          .min(1, t('setting.modelNameMessage'))
          .refine(
            (name) =>
              Boolean(model) ||
              !existingNames.some(
                (existing) =>
                  existing.trim().toLowerCase() === name.toLowerCase(),
              ),
            t('setting.modelNameDuplicate'),
          ),
        model_type: z
          .array(z.string())
          .min(1, t('setting.modelTypeMessage'))
          .refine(
            (types) =>
              !['somark', 'opendataloader'].includes(
                provider.name.toLowerCase(),
              ) ||
              !types.includes('doc_parse') ||
              types.includes('ocr'),
            t('admin.platformModelsPage.parserNeedsOcr'),
          ),
        max_tokens: z.coerce
          .number()
          .int(t('admin.platformModelsPage.maxTokensMin'))
          .min(1, t('admin.platformModelsPage.maxTokensMin')),
        is_tools: z.boolean(),
      }),
    [existingNames, model, provider.name, t],
  );
  type FormValues = z.infer<typeof schema>;

  // Where the tool support value comes from. Without one, nothing is saved
  // and the built-in catalog decides, as for tenant models.
  const [toolsSource, setToolsSource] = useState<
    'saved' | 'discovered' | 'admin' | null
  >(typeof model?.extra?.is_tools === 'boolean' ? 'saved' : null);
  const form = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: {
      model_name: model?.model_name ?? '',
      model_type: model?.model_type ?? [],
      max_tokens: model?.max_tokens ?? DefaultMaxTokens,
      is_tools: model?.extra?.is_tools === true,
    },
  });

  const filledRef = useRef<{ model_type?: string[]; max_tokens?: number }>({});
  // Whether the name input has focus; see the name field below.
  const [typingName, setTypingName] = useState(false);
  // Only the name input counts: React focus events also bubble from the
  // picker's search box, which renders in a portal.
  const isNameInput = (target: EventTarget) =>
    target instanceof HTMLInputElement && target.name === 'model_name';
  const handleNameFocus = useCallback((event: FocusEvent<HTMLElement>) => {
    if (isNameInput(event.target)) setTypingName(true);
  }, []);
  const handleNameBlur = useCallback((event: FocusEvent<HTMLElement>) => {
    if (isNameInput(event.target)) setTypingName(false);
  }, []);

  const discovered = useMemo(
    () => discoverMutation.data ?? [],
    [discoverMutation.data],
  );
  const discoveredOptions = useMemo(
    () =>
      discovered
        .filter((item) => item.name)
        .map((item) => ({ label: item.name, value: item.name })),
    [discovered],
  );

  // Listing and saving share one lock, taken before any await: one request
  // at a time, and the dialog never closes on a save while listing runs.
  const actionRef = useRef(false);
  const runExclusive = useCallback(async (action: () => Promise<void>) => {
    if (actionRef.current) return;
    actionRef.current = true;
    try {
      await action();
    } finally {
      actionRef.current = false;
    }
  }, []);
  const busy = saveMutation.isPending || discoverMutation.isPending;
  const busyEvents = useBlockBusyEvents(busy);

  const discover = useCallback(async () => {
    try {
      const items = await discoverMutation.mutateAsync({
        instance_id: instance.id,
        use_saved_credentials: true,
      });
      if (!items.some((item) => item.name)) {
        message.info(t('admin.platformModelsPage.noDiscoveredModels'));
      }
    } catch {
      // Reported by the admin request layer.
    }
  }, [discoverMutation, instance.id, t]);
  const handleDiscover = useCallback(
    () => runExclusive(discover),
    [runExclusive, discover],
  );

  const handlePickDiscovered = useCallback(
    (name: string) => {
      form.setValue('model_name', name, { shouldValidate: true });
      const item = discovered.find((candidate) => candidate.name === name);
      // Values filled from an earlier pick do not describe another model;
      // those the admin has not changed since go back to the defaults.
      const previous = filledRef.current;
      filledRef.current = {};
      if (
        previous.model_type &&
        isEqual(form.getValues('model_type'), previous.model_type)
      ) {
        form.setValue('model_type', [], { shouldValidate: true });
      }
      if (
        previous.max_tokens !== undefined &&
        form.getValues('max_tokens') === previous.max_tokens
      ) {
        form.setValue('max_tokens', DefaultMaxTokens);
      }
      if (!item) {
        if (toolsSource === 'discovered') {
          form.setValue('is_tools', false);
          setToolsSource(null);
        }
        return;
      }
      const types = (item.model_types ?? []).filter((type) =>
        (PlatformModelTypes as readonly string[]).includes(type),
      );
      if (types.length > 0) {
        form.setValue('model_type', types, { shouldValidate: true });
        filledRef.current.model_type = types;
      }
      if (item.max_tokens && item.max_tokens > 0) {
        form.setValue('max_tokens', item.max_tokens);
        filledRef.current.max_tokens = item.max_tokens;
      }
      const tools = Array.isArray(item.features)
        ? item.features.includes('is_tools')
        : item.tools === 'supported' || undefined;
      if (tools !== undefined) {
        form.setValue('is_tools', tools);
        setToolsSource('discovered');
      } else if (toolsSource === 'discovered') {
        // The previous pick's value does not describe this model.
        form.setValue('is_tools', false);
        setToolsSource(null);
      }
    },
    [discovered, form, toolsSource],
  );

  // Enter in a field submits too.
  const save = useCallback(
    async (values: FormValues) => {
      try {
        await saveMutation.mutateAsync({
          modelId: model?.model_id,
          params: {
            model_name: values.model_name.trim(),
            model_type: values.model_type,
            max_tokens: values.max_tokens,
            ...(toolsSource ? { extra: { is_tools: values.is_tools } } : {}),
          },
        });
        message.success(t(model ? 'message.modified' : 'message.created'));
        onOpenChange(false);
      } catch {
        // The admin request layer already reported the error; keep the input.
      }
    },
    [saveMutation, model, toolsSource, onOpenChange, t],
  );
  const handleSubmit = useCallback(
    (values: FormValues) => runExclusive(() => save(values)),
    [runExclusive, save],
  );

  const handleToolsChange = useCallback(
    (checked: boolean) => {
      setToolsSource('admin');
      form.setValue('is_tools', checked);
    },
    [form],
  );

  const handleCancel = onCancel;

  return (
    <Form {...form}>
      {/* The schema reports every invalid value in the form's own,
          translated messages instead of the browser's bubbles. */}
      <form
        noValidate
        className="px-6 space-y-6"
        onSubmit={form.handleSubmit(handleSubmit)}
      >
        <fieldset disabled={busy} className="space-y-6" {...busyEvents}>
          {/* The picker is the form control itself so its label and errors
            stay associated; discovery sits beside it. */}
          <div
            className="flex items-start gap-2 min-w-0"
            onFocus={handleNameFocus}
            onBlur={handleNameBlur}
          >
            <RAGFlowFormItem
              name="model_name"
              label={t('setting.modelName')}
              required
              className="flex-1 min-w-0"
            >
              {(field) =>
                // While the admin types a name, discovered models do not swap
                // the input away under the cursor; the list appears on blur.
                model || discoveredOptions.length === 0 || typingName ? (
                  <Input
                    {...field}
                    disabled={Boolean(model)}
                    placeholder={t('setting.modelNameMessage')}
                    autoComplete="off"
                  />
                ) : (
                  <SelectWithSearch
                    ref={field.ref}
                    ariaLabel={t('setting.modelName')}
                    value={field.value}
                    onChange={handlePickDiscovered}
                    options={discoveredOptions}
                    allowCustomValue
                    placeholder={t('setting.modelNameMessage')}
                  />
                )
              }
            </RAGFlowFormItem>
            {!model && (
              <BusyButton
                type="button"
                variant="outline"
                // Same height and top as the name field beside it.
                className="mt-[1.375rem] h-8 shrink-0 dark:border-border-button"
                onClick={handleDiscover}
                busy={busy}
              >
                {t('setting.listModels')}
              </BusyButton>
            )}
          </div>

          <RAGFlowFormItem
            name="model_type"
            label={t('setting.modelType')}
            required
          >
            {(field) => (
              // Takes focus when no type is chosen, so the error is found.
              <div
                ref={field.ref}
                tabIndex={-1}
                role="group"
                aria-label={t('setting.modelType')}
                className="flex flex-wrap gap-x-6 gap-y-3 outline-none"
              >
                {PlatformModelTypes.map((type) => (
                  <ModelTypeCheckbox
                    key={type}
                    type={type}
                    selected={field.value as string[]}
                    onChange={field.onChange}
                  />
                ))}
              </div>
            )}
          </RAGFlowFormItem>

          <RAGFlowFormItem name="max_tokens" label={t('setting.maxTokens')}>
            {(field) => <Input {...field} type="number" min={1} />}
          </RAGFlowFormItem>

          <RAGFlowFormItem
            name="is_tools"
            label={t('admin.platformModelsPage.toolCalling')}
            tooltip={t('admin.platformModelsPage.toolCallingHint')}
            description={
              toolsSource
                ? undefined
                : t('admin.platformModelsPage.toolCallingFromCatalog')
            }
            horizontal
          >
            {(field) => (
              <Switch
                checked={field.value}
                onCheckedChange={handleToolsChange}
                aria-label={t('admin.platformModelsPage.toolCalling')}
              />
            )}
          </RAGFlowFormItem>
        </fieldset>
        <DialogFooter className="gap-4 py-4">
          <Button
            type="button"
            variant="outline"
            className="px-4 h-10 dark:border-border-button"
            onClick={handleCancel}
            disabled={busy}
          >
            {t('admin.cancel')}
          </Button>
          <BusyButton type="submit" className="px-4 h-10" busy={busy}>
            {t('admin.confirm')}
          </BusyButton>
        </DialogFooter>
      </form>
    </Form>
  );
}

function ModelTypeCheckbox({
  type,
  selected,
  onChange,
}: {
  type: string;
  selected: string[];
  onChange: (value: string[]) => void;
}) {
  const { t } = useTranslation();
  const id = `platform-model-type-${type}`;
  const handleCheckedChange = useCallback(
    (next: boolean | 'indeterminate') =>
      onChange(
        next === true
          ? [...selected, type]
          : selected.filter((value) => value !== type),
      ),
    [onChange, selected, type],
  );
  return (
    <div className="flex items-center gap-2">
      <Checkbox
        id={id}
        checked={selected.includes(type)}
        onCheckedChange={handleCheckedChange}
      />
      <Label htmlFor={id} className="font-normal">
        {t(PlatformModelTypeLabelKeys[type])}
      </Label>
    </div>
  );
}
