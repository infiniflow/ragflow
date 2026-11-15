import { FormContainer } from '@/components/form-container';
import { RAGFlowFormItem } from '@/components/ragflow-form';
import { TopNFormField } from '@/components/top-n-item';
import { Form } from '@/components/ui/form';
import { SelectWithSearch } from '@/components/originui/select-with-search';
import { useTranslate } from '@/hooks/common-hooks';
import i18n from '@/locales/config';
import { zodResolver } from '@hookform/resolvers/zod';
import { memo, useEffect, useMemo } from 'react';
import { useForm, useFormContext, useWatch } from 'react-hook-form';
import { z } from 'zod';
import {
  Search1APIChannel,
  Search1APIServices,
  initialSearch1APISearchValues,
} from '../../constant';
import { useFormValues } from '../../hooks/use-form-values';
import { useWatchFormChange } from '../../hooks/use-watch-form-change';
import { INextOperatorForm } from '../../interface';
import { buildOutputList } from '../../utils/build-output-list';
import { ApiKeyField } from '../components/api-key-field';
import { FormWrapper } from '../components/form-wrapper';
import { Output } from '../components/output';
import { QueryVariable } from '../components/query-variable';

const Search1APIChannelLabelKeys: Record<Search1APIChannel, string> = {
  [Search1APIChannel.General]: 'search1APIChannelGeneral',
  [Search1APIChannel.News]: 'search1APIChannelNews',
};

// Service names are brands, so they are shown as-is rather than translated.
const Search1APIServiceLabels: Record<string, string> = {
  google: 'Google',
  bing: 'Bing',
  bingcn: 'Bing CN',
  duckduckgo: 'DuckDuckGo',
  yahoo: 'Yahoo',
  yandex: 'Yandex',
  youtube: 'YouTube',
  x: 'X',
  reddit: 'Reddit',
  github: 'GitHub',
  arxiv: 'arXiv',
  wechat: 'WeChat',
  bilibili: 'Bilibili',
  imdb: 'IMDb',
  wikipedia: 'Wikipedia',
  baidu: 'Baidu',
  '360': '360',
  quark: 'Quark',
  hackernews: 'Hacker News',
  reuters: 'Reuters',
};

export const Search1APIFormPartialSchema = {
  api_key: z.string().min(1, i18n.t('flow.apiKeyRequired')),
  channel: z.string().optional(),
  search_service: z.string().optional(),
  top_n: z.coerce.number(),
};

const FormSchema = z.object({
  query: z.string(),
  ...Search1APIFormPartialSchema,
});

export function Search1APIWidgets() {
  const { t } = useTranslate('flow');
  const form = useFormContext();
  const channel: Search1APIChannel =
    useWatch({ control: form.control, name: 'channel' }) ||
    Search1APIChannel.General;
  const services = useMemo(() => Search1APIServices[channel] ?? [], [channel]);

  const channelOptions = useMemo(
    () =>
      Object.values(Search1APIChannel).map((x) => ({
        value: x,
        label: t(Search1APIChannelLabelKeys[x]),
      })),
    [t],
  );

  const serviceOptions = useMemo(
    () =>
      services.map((x) => ({
        value: x,
        label: Search1APIServiceLabels[x] ?? x,
      })),
    [services],
  );

  // Each channel offers its own services, so a service the new channel lacks
  // falls back to the channel's first one.
  useEffect(() => {
    const current = form.getValues('search_service');
    if (services.length > 0 && !services.includes(current)) {
      form.setValue('search_service', services[0], { shouldDirty: true });
    }
  }, [form, services]);

  return (
    <>
      <ApiKeyField
        placeholder={t('search1APIApiKeyTip')}
        required
      ></ApiKeyField>
      <RAGFlowFormItem
        name="channel"
        label={t('channel')}
        tooltip={t('search1APIChannelTip')}
      >
        <SelectWithSearch options={channelOptions} />
      </RAGFlowFormItem>
      <RAGFlowFormItem
        name="search_service"
        label={t('search1APIService')}
        tooltip={t('search1APIServiceTip')}
      >
        <SelectWithSearch options={serviceOptions} />
      </RAGFlowFormItem>
      <TopNFormField max={50}></TopNFormField>
    </>
  );
}

const Search1APIOutputList = buildOutputList(
  initialSearch1APISearchValues.outputs,
);

function Search1APIForm({ node }: INextOperatorForm) {
  const defaultValues = useFormValues(initialSearch1APISearchValues, node);

  const form = useForm<z.infer<typeof FormSchema>>({
    defaultValues,
    resolver: zodResolver(FormSchema),
    mode: 'onChange',
  });

  useWatchFormChange(node?.id, form);

  return (
    <Form {...form}>
      <FormWrapper>
        <FormContainer>
          <QueryVariable></QueryVariable>
          <Search1APIWidgets></Search1APIWidgets>
        </FormContainer>
      </FormWrapper>
      <div className="p-5">
        <Output list={Search1APIOutputList}></Output>
      </div>
    </Form>
  );
}

export default memo(Search1APIForm);
