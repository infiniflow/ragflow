import { RAGFlowFormItem } from '@/components/ragflow-form';
import { Button, ButtonLoading } from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Form } from '@/components/ui/form';
import { Input } from '@/components/ui/input';
import { RAGFlowSelect } from '@/components/ui/select';
import { Switch } from '@/components/ui/switch';
import { LLMFactory } from '@/constants/llm';
import { VerifyResult } from '@/pages/user-setting/setting-model/hooks';
import { zodResolver } from '@hookform/resolvers/zod';
import { memo, useMemo } from 'react';
import { useForm } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { z } from 'zod';
import { LLMHeader } from '../../components/llm-header';
import VerifyButton from '../verify-button';

const IMAGE_FORMATS = ['url', 'base64', 'none'] as const;
const FORMULA_FORMATS = ['latex', 'mathml', 'ascii'] as const;
const TABLE_FORMATS = ['html', 'markdown', 'image'] as const;
const CS_FORMATS = ['image'] as const;
const FORMAT_LABELS = {
  url: 'URL',
  base64: 'Base64',
  none: 'None',
  latex: 'LaTeX',
  mathml: 'MathML',
  ascii: 'ASCII',
  html: 'HTML',
  markdown: 'Markdown',
  image: 'Image',
} as const;

export type SoMarkFormValues = {
  instance_name: string;
  llm_name: string;
  somark_base_url: string;
  somark_api_key?: string;
  somark_image_format: (typeof IMAGE_FORMATS)[number];
  somark_formula_format: (typeof FORMULA_FORMATS)[number];
  somark_table_format: (typeof TABLE_FORMATS)[number];
  somark_cs_format: (typeof CS_FORMATS)[number];
  somark_enable_text_cross_page: boolean;
  somark_enable_table_cross_page: boolean;
  somark_enable_title_level_recognition: boolean;
  somark_enable_inline_image: boolean;
  somark_enable_table_image: boolean;
  somark_enable_image_understanding: boolean;
  somark_keep_header_footer: boolean;
};

export interface IModalProps<T> {
  visible: boolean;
  hideModal: () => void;
  onOk?: (data: T) => Promise<boolean>;
  onVerify?: (
    postBody: any,
  ) => Promise<boolean | void | VerifyResult | undefined>;
  loading?: boolean;
}

const SectionTitle = ({ children }: { children: React.ReactNode }) => (
  <div className="text-sm font-semibold text-muted-foreground border-b pb-1">
    {children}
  </div>
);

const buildFormatOptions = <T extends keyof typeof FORMAT_LABELS>(
  formats: readonly T[],
) => formats.map((value) => ({ label: FORMAT_LABELS[value], value }));

const SoMarkModal = ({
  visible,
  hideModal,
  onOk,
  onVerify,
  loading,
}: IModalProps<SoMarkFormValues>) => {
  const { t } = useTranslation();

  const FormSchema = useMemo(
    () =>
      z.object({
        instance_name: z.string().min(1, {
          message: t('setting.instanceNameMessage'),
        }),
        llm_name: z.string().min(1, {
          message: t('setting.somark.modelNameMessage'),
        }),
        somark_base_url: z.string().min(1, {
          message: t('setting.somark.baseUrlMessage'),
        }),
        somark_api_key: z.string().optional(),
        somark_image_format: z.enum(IMAGE_FORMATS),
        somark_formula_format: z.enum(FORMULA_FORMATS),
        somark_table_format: z.enum(TABLE_FORMATS),
        somark_cs_format: z.enum(CS_FORMATS),
        somark_enable_text_cross_page: z.boolean(),
        somark_enable_table_cross_page: z.boolean(),
        somark_enable_title_level_recognition: z.boolean(),
        somark_enable_inline_image: z.boolean(),
        somark_enable_table_image: z.boolean(),
        somark_enable_image_understanding: z.boolean(),
        somark_keep_header_footer: z.boolean(),
      }),
    [t],
  );

  const imageFormatOptions = useMemo(
    () => buildFormatOptions(IMAGE_FORMATS),
    [],
  );
  const formulaFormatOptions = useMemo(
    () => buildFormatOptions(FORMULA_FORMATS),
    [],
  );
  const tableFormatOptions = useMemo(
    () => buildFormatOptions(TABLE_FORMATS),
    [],
  );
  const csFormatOptions = useMemo(() => buildFormatOptions(CS_FORMATS), []);

  const form = useForm<SoMarkFormValues>({
    resolver: zodResolver(FormSchema),
    defaultValues: {
      instance_name: '',
      llm_name: '',
      somark_base_url: '',
      somark_api_key: '',
      somark_image_format: 'url',
      somark_formula_format: 'latex',
      somark_table_format: 'html',
      somark_cs_format: 'image',
      somark_enable_text_cross_page: false,
      somark_enable_table_cross_page: false,
      somark_enable_title_level_recognition: false,
      somark_enable_inline_image: false,
      somark_enable_table_image: true,
      somark_enable_image_understanding: true,
      somark_keep_header_footer: false,
    },
  });

  const handleOk = async (values: SoMarkFormValues) => {
    const ret = await onOk?.(values as any);
    if (ret) {
      hideModal?.();
    }
  };

  return (
    <Dialog open={visible} onOpenChange={hideModal}>
      <DialogContent className="max-h-[80vh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle>
            <LLMHeader name={LLMFactory.SoMark} />
          </DialogTitle>
        </DialogHeader>
        <Form {...form}>
          <form
            onSubmit={form.handleSubmit(handleOk)}
            className="space-y-5"
            id="somark-form"
          >
            <RAGFlowFormItem
              name="instance_name"
              label={t('setting.instanceName')}
              tooltip={t('setting.instanceNameTip')}
              required
            >
              <Input placeholder={t('setting.instanceNameMessage')} />
            </RAGFlowFormItem>
            <RAGFlowFormItem
              name="llm_name"
              label={t('setting.modelName')}
              required
            >
              <Input placeholder="somark-from-env-1" />
            </RAGFlowFormItem>
            <RAGFlowFormItem
              name="somark_base_url"
              label={t('setting.somark.baseUrl')}
              required
            >
              <Input placeholder={t('setting.somark.baseUrlPlaceholder')} />
            </RAGFlowFormItem>
            <RAGFlowFormItem
              name="somark_api_key"
              label={t('setting.somark.apiKey')}
            >
              <Input
                type="password"
                placeholder={t('setting.somark.apiKeyPlaceholder')}
              />
            </RAGFlowFormItem>

            <SectionTitle>
              {t('setting.somark.sectionElementFormats')}
            </SectionTitle>
            <RAGFlowFormItem
              name="somark_image_format"
              label={t('setting.somark.imageFormat')}
            >
              {(field) => (
                <RAGFlowSelect
                  value={field.value}
                  onChange={field.onChange}
                  options={imageFormatOptions}
                />
              )}
            </RAGFlowFormItem>
            <RAGFlowFormItem
              name="somark_formula_format"
              label={t('setting.somark.formulaFormat')}
            >
              {(field) => (
                <RAGFlowSelect
                  value={field.value}
                  onChange={field.onChange}
                  options={formulaFormatOptions}
                />
              )}
            </RAGFlowFormItem>
            <RAGFlowFormItem
              name="somark_table_format"
              label={t('setting.somark.tableFormat')}
            >
              {(field) => (
                <RAGFlowSelect
                  value={field.value}
                  onChange={field.onChange}
                  options={tableFormatOptions}
                />
              )}
            </RAGFlowFormItem>
            <RAGFlowFormItem
              name="somark_cs_format"
              label={t('setting.somark.csFormat')}
            >
              {(field) => (
                <RAGFlowSelect
                  value={field.value}
                  onChange={field.onChange}
                  options={csFormatOptions}
                />
              )}
            </RAGFlowFormItem>

            <SectionTitle>
              {t('setting.somark.sectionFeatureConfig')}
            </SectionTitle>
            <RAGFlowFormItem
              name="somark_enable_text_cross_page"
              label={t('setting.somark.enableTextCrossPage')}
              labelClassName="!mb-0"
            >
              {(field) => (
                <Switch
                  checked={field.value}
                  onCheckedChange={field.onChange}
                />
              )}
            </RAGFlowFormItem>
            <RAGFlowFormItem
              name="somark_enable_table_cross_page"
              label={t('setting.somark.enableTableCrossPage')}
              labelClassName="!mb-0"
            >
              {(field) => (
                <Switch
                  checked={field.value}
                  onCheckedChange={field.onChange}
                />
              )}
            </RAGFlowFormItem>
            <RAGFlowFormItem
              name="somark_enable_title_level_recognition"
              label={t('setting.somark.enableTitleLevelRecognition')}
              labelClassName="!mb-0"
            >
              {(field) => (
                <Switch
                  checked={field.value}
                  onCheckedChange={field.onChange}
                />
              )}
            </RAGFlowFormItem>
            <RAGFlowFormItem
              name="somark_enable_inline_image"
              label={t('setting.somark.enableInlineImage')}
              labelClassName="!mb-0"
            >
              {(field) => (
                <Switch
                  checked={field.value}
                  onCheckedChange={field.onChange}
                />
              )}
            </RAGFlowFormItem>
            <RAGFlowFormItem
              name="somark_enable_table_image"
              label={t('setting.somark.enableTableImage')}
              labelClassName="!mb-0"
            >
              {(field) => (
                <Switch
                  checked={field.value}
                  onCheckedChange={field.onChange}
                />
              )}
            </RAGFlowFormItem>
            <RAGFlowFormItem
              name="somark_enable_image_understanding"
              label={t('setting.somark.enableImageUnderstanding')}
              labelClassName="!mb-0"
            >
              {(field) => (
                <Switch
                  checked={field.value}
                  onCheckedChange={field.onChange}
                />
              )}
            </RAGFlowFormItem>
            <RAGFlowFormItem
              name="somark_keep_header_footer"
              label={t('setting.somark.keepHeaderFooter')}
              labelClassName="!mb-0"
            >
              {(field) => (
                <Switch
                  checked={field.value}
                  onCheckedChange={field.onChange}
                />
              )}
            </RAGFlowFormItem>

            {onVerify && (
              <VerifyButton
                onVerify={onVerify as (postBody: any) => Promise<VerifyResult>}
                isAbsolute={false}
                validLabel={t('setting.somark.verifyPassed')}
                invalidLabel={t('setting.somark.verifyFailed')}
              />
            )}
          </form>
        </Form>
        <DialogFooter className="flex justify-end space-x-2">
          <Button type="button" variant="secondary" onClick={hideModal}>
            {t('common.cancel')}
          </Button>
          <ButtonLoading type="submit" form="somark-form" loading={loading}>
            {t('common.add')}
          </ButtonLoading>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
};

export default memo(SoMarkModal);
