import queritLogo from '@/assets/querit.png';
import tavilyLogo from '@/assets/svg/tavily.svg';
import { RAGFlowSelect } from '@/components/ui/select';
import { WebSearchProvider } from '@/constants/chat';
import { useTranslate } from '@/hooks/common-hooks';
import { prefixName } from '@/utils/form';
import { useFormContext, useWatch } from 'react-hook-form';
import PasswordInput from './originui/password-input';
import {
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from './ui/form';

interface IProps {
  prefix?: string;
}

const providerOptions = [
  {
    name: 'Tavily',
    logo: tavilyLogo,
    value: WebSearchProvider.Tavily,
  },
  {
    name: 'Querit',
    logo: queritLogo,
    value: WebSearchProvider.Querit,
  },
]
  .sort((left, right) => left.name.localeCompare(right.name))
  .map(({ name, logo, value }) => ({
    label: (
      <span className="flex items-center gap-2">
        <img
          src={logo}
          alt=""
          aria-hidden="true"
          className="size-4 shrink-0 object-contain"
        />
        {name}
      </span>
    ),
    value,
  }));

const providerKeyConfig = {
  [WebSearchProvider.Tavily]: {
    name: 'prompt_config.tavily_api_key',
    label: 'Tavily API Key',
    tip: 'tavilyApiKeyTip',
    placeholder: 'tavilyApiKeyMessage',
    helpUrl: 'https://app.tavily.com/home',
  },
  [WebSearchProvider.Querit]: {
    name: 'prompt_config.querit_api_key',
    label: 'Querit API Key',
    tip: 'queritApiKeyTip',
    placeholder: 'queritApiKeyMessage',
    helpUrl: 'https://querit.ai',
  },
} as const;

export function WebSearchFormField({ prefix = '' }: IProps) {
  const form = useFormContext();
  const { t } = useTranslate('chat');
  const providerName = prefixName(prefix, 'prompt_config.web_search_provider');
  const selectedProvider = useWatch({
    control: form.control,
    name: providerName,
  });
  const keyConfig = providerKeyConfig[selectedProvider as WebSearchProvider];

  return (
    <>
      <FormField
        control={form.control}
        name={providerName}
        render={({ field }) => (
          <FormItem>
            <FormLabel tooltip={t('webSearchProviderTip')}>
              {t('webSearchProvider')}
            </FormLabel>
            <FormControl>
              <RAGFlowSelect
                {...field}
                value={field.value}
                options={providerOptions}
                placeholder={t('webSearchProviderPlaceholder')}
                triggerTestId="web-search-provider"
                optionTestIdPrefix="web-search-provider-option"
              />
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />
      {keyConfig && (
        <FormField
          key={selectedProvider}
          control={form.control}
          name={prefixName(prefix, keyConfig.name)}
          render={({ field }) => (
            <FormItem>
              <FormLabel tooltip={t(keyConfig.tip)}>
                {keyConfig.label}
              </FormLabel>
              <FormControl>
                <PasswordInput
                  {...field}
                  value={field.value ?? ''}
                  placeholder={t(keyConfig.placeholder)}
                  autoComplete="new-password"
                />
              </FormControl>
              <FormDescription>
                <a href={keyConfig.helpUrl} target="_blank" rel="noreferrer">
                  {t('tavilyApiKeyHelp')}
                </a>
              </FormDescription>
              <FormMessage />
            </FormItem>
          )}
        />
      )}
    </>
  );
}
