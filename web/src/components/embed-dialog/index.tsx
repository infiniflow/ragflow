import CopyToClipboard from '@/components/copy-to-clipboard';
import HightLightMarkdown from '@/components/highlight-markdown';
import { SelectWithSearch } from '@/components/originui/select-with-search';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { Switch } from '@/components/ui/switch';
import { SharedFrom } from '@/constants/chat';
import {
  LanguageAbbreviation,
  LanguageAbbreviationMap,
} from '@/constants/common';
import { useTranslate } from '@/hooks/common-hooks';
import { IModalProps } from '@/interfaces/common';
import { Routes } from '@/routes';
import { zodResolver } from '@hookform/resolvers/zod';
import { memo, useCallback, useMemo } from 'react';
import { useForm, useWatch } from 'react-hook-form';
import { z } from 'zod';

const FormSchema = z.object({
  visibleAvatar: z.boolean(),
  locale: z.string(),
});

type IProps = IModalProps<any> & {
  token: string;
  from: SharedFrom;
  beta: string;
  isAgent: boolean;
};

function EmbedDialog({
  hideModal,
  token = '',
  from,
  beta = '',
  isAgent,
}: IProps) {
  const { t } = useTranslate('chat');

  const form = useForm<z.infer<typeof FormSchema>>({
    resolver: zodResolver(FormSchema),
    defaultValues: {
      visibleAvatar: false,
      locale: '',
    },
  });

  const values = useWatch({ control: form.control });

  const languageOptions = useMemo(() => {
    return Object.values(LanguageAbbreviation).map((x) => ({
      label: LanguageAbbreviationMap[x],
      value: x,
    }));
  }, []);

  const generateIframeSrc = useCallback(() => {
    const { visibleAvatar, locale } = values;
    let src = `${location.origin}${from === SharedFrom.Agent ? Routes.AgentShare : Routes.ChatShare}?shared_id=${token}&from=${from}&auth=${beta}`;
    if (visibleAvatar) {
      src += '&visible_avatar=1';
    }
    if (locale) {
      src += `&locale=${locale}`;
    }
    return src;
  }, [beta, from, token, values]);

  const text = useMemo(() => {
    const iframeSrc = generateIframeSrc();
    return `
  ~~~ html
  <iframe
  src="${iframeSrc}"
  style="width: 100%; height: 100%; min-height: 600px"
  frameborder="0"
>
</iframe>
~~~
  `;
  }, [generateIframeSrc]);

  return (
    <Dialog open onOpenChange={hideModal}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>
            {t('embedIntoSite', { keyPrefix: 'common' })}
          </DialogTitle>
        </DialogHeader>
        <section className="w-full overflow-auto space-y-5 text-sm text-text-secondary">
          <Form {...form}>
            <form className="space-y-5">
              <FormField
                control={form.control}
                name="visibleAvatar"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('avatarHidden')}</FormLabel>
                    <FormControl>
                      <Switch
                        checked={field.value}
                        onCheckedChange={field.onChange}
                      ></Switch>
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name="locale"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('locale')}</FormLabel>
                    <FormControl>
                      <SelectWithSearch
                        {...field}
                        options={languageOptions}
                      ></SelectWithSearch>
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </form>
          </Form>
          <div>
            <span>Embed code</span>
            <HightLightMarkdown>{text}</HightLightMarkdown>
          </div>
          <div className=" font-medium mt-4 mb-1">
            {t(isAgent ? 'flow' : 'chat', { keyPrefix: 'header' })}
            <span className="ml-1 inline-block">ID</span>
          </div>
          <div className="bg-bg-card rounded-lg flex justify-between p-2">
            <span>{token} </span>
            <CopyToClipboard text={token}></CopyToClipboard>
          </div>
          <a
            className="cursor-pointer text-accent-primary inline-block"
            href={
              isAgent
                ? 'https://ragflow.io/docs/dev/http_api_reference#create-session-with-agent'
                : 'https://ragflow.io/docs/dev/http_api_reference#create-session-with-chat-assistant'
            }
            target="_blank"
            rel="noreferrer"
          >
            {t('howUseId', { keyPrefix: isAgent ? 'flow' : 'chat' })}
          </a>
        </section>
      </DialogContent>
    </Dialog>
  );
}

export default memo(EmbedDialog);
