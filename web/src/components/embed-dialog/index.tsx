import CopyToClipboard from '@/components/copy-to-clipboard';
import HighLightMarkdown from '@/components/highlight-markdown';
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
import { Label } from '@/components/ui/label';
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group';
import { Switch } from '@/components/ui/switch';
import { SharedFrom } from '@/constants/chat';
import {
  LanguageAbbreviation,
  LanguageAbbreviationMap,
  ThemeEnum,
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
  embedType: z.enum(['fullscreen', 'widget']),
  enableStreaming: z.boolean(),
  theme: z.enum([ThemeEnum.Light, ThemeEnum.Dark]),
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
      embedType: 'fullscreen' as const,
      enableStreaming: false,
      theme: ThemeEnum.Light,
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
    const { visibleAvatar, locale, embedType, enableStreaming, theme } = values;
    const baseRoute =
      embedType === 'widget'
        ? Routes.ChatWidget
        : from === SharedFrom.Agent
          ? Routes.AgentShare
          : Routes.ChatShare;
    let src = `${location.origin}${baseRoute}?shared_id=${token}&from=${from}&auth=${beta}`;
    if (visibleAvatar) {
      src += '&visible_avatar=1';
    }
    if (locale) {
      src += `&locale=${locale}`;
    }
    if (enableStreaming) {
      src += '&streaming=true';
    }
    if (theme && embedType === 'fullscreen') {
      src += `&theme=${theme}`;
    }
    return src;
  }, [beta, from, token, values]);

  const text = useMemo(() => {
    const iframeSrc = generateIframeSrc();
    const { embedType } = values;

    if (embedType === 'widget') {
      const { enableStreaming } = values;
      const streamingParam = enableStreaming
        ? '&streaming=true'
        : '&streaming=false';
      return `
  ~~~ html
  <iframe src="${iframeSrc}&mode=master${streamingParam}"
    style="position:fixed;bottom:0;right:0;width:100px;height:100px;border:none;background:transparent;z-index:9999"
    frameborder="0" allow="microphone;camera"></iframe>
  <script>
  window.addEventListener('message',e=>{
    if(e.origin!=='${location.origin.replace(/:\d+/, ':9222')}')return;
    if(e.data.type==='CREATE_CHAT_WINDOW'){
      if(document.getElementById('chat-win'))return;
      const i=document.createElement('iframe');
      i.id='chat-win';i.src=e.data.src;
      i.style.cssText='position:fixed;bottom:104px;right:24px;width:380px;height:500px;border:none;background:transparent;z-index:9998;display:none';
      i.frameBorder='0';i.allow='microphone;camera';
      document.body.appendChild(i);
    }else if(e.data.type==='TOGGLE_CHAT'){
      const w=document.getElementById('chat-win');
      if(w)w.style.display=e.data.isOpen?'block':'none';
    }else if(e.data.type==='SCROLL_PASSTHROUGH')window.scrollBy(0,e.data.deltaY);
  });
  </script>
~~~
  `;
    } else {
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
    }
  }, [generateIframeSrc, values]);

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
                name="embedType"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>Embed Type</FormLabel>
                    <FormControl>
                      <RadioGroup
                        onValueChange={field.onChange}
                        value={field.value}
                        className="flex flex-col space-y-2"
                      >
                        <div className="flex items-center space-x-2">
                          <RadioGroupItem value="fullscreen" id="fullscreen" />
                          <Label htmlFor="fullscreen" className="text-sm">
                            Fullscreen Chat (Traditional iframe)
                          </Label>
                        </div>
                        <div className="flex items-center space-x-2">
                          <RadioGroupItem value="widget" id="widget" />
                          <Label htmlFor="widget" className="text-sm">
                            Floating Widget (Intercom-style)
                          </Label>
                        </div>
                      </RadioGroup>
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              {values.embedType === 'fullscreen' && (
                <FormField
                  control={form.control}
                  name="theme"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>Theme</FormLabel>
                      <FormControl>
                        <RadioGroup
                          onValueChange={field.onChange}
                          value={field.value}
                          className="flex flex-row space-x-4"
                        >
                          <div className="flex items-center space-x-2">
                            <RadioGroupItem
                              value={ThemeEnum.Light}
                              id="light"
                            />
                            <Label htmlFor="light" className="text-sm">
                              Light
                            </Label>
                          </div>
                          <div className="flex items-center space-x-2">
                            <RadioGroupItem value={ThemeEnum.Dark} id="dark" />
                            <Label htmlFor="dark" className="text-sm">
                              Dark
                            </Label>
                          </div>
                        </RadioGroup>
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              )}
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
              {values.embedType === 'widget' && (
                <FormField
                  control={form.control}
                  name="enableStreaming"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>Enable Streaming Responses</FormLabel>
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
              )}
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
          <div className="max-h-[350px] overflow-auto">
            <span>{t('embedCode', { keyPrefix: 'search' })}</span>
            <div className="max-h-full overflow-y-auto">
              <HighLightMarkdown>{text}</HighLightMarkdown>
            </div>
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
