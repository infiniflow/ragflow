import CopyToClipboard from '@/components/copy-to-clipboard';
import { SelectWithSearch } from '@/components/originui/select-with-search';
import { Button } from '@/components/ui/button';
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
import { SharedFrom } from '@/constants/chat';
import {
  LanguageAbbreviation,
  LanguageAbbreviationMap,
  ThemeEnum,
} from '@/constants/common';
import { IModalProps } from '@/interfaces/common';
import { Routes } from '@/routes';
import { zodResolver } from '@hookform/resolvers/zod';
import { isEmpty, trim } from 'lodash';
import { ExternalLink } from 'lucide-react';
import { memo, useCallback, useMemo } from 'react';
import { useForm, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { Prism as SyntaxHighlighter } from 'react-syntax-highlighter';
import {
  oneDark,
  oneLight,
} from 'react-syntax-highlighter/dist/esm/styles/prism';
import { z } from 'zod';
import { RAGFlowFormItem } from '../ragflow-form';
import { SwitchFormField } from '../switch-fom-field';
import { useIsDarkTheme } from '../theme-provider';
import { Input } from '../ui/input';

const FormSchema = z.object({
  visibleAvatar: z.boolean(),
  published: z.boolean(),
  locale: z.string(),
  embedType: z.enum(['fullscreen', 'widget']),
  enableStreaming: z.boolean(),
  theme: z.enum([ThemeEnum.Light, ThemeEnum.Dark]),
  userId: z.string().optional(),
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
  visible,
}: IProps) {
  const { t } = useTranslation();
  const isDarkTheme = useIsDarkTheme();

  const form = useForm<z.infer<typeof FormSchema>>({
    resolver: zodResolver(FormSchema),
    defaultValues: {
      visibleAvatar: false,
      published: false,
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
    const {
      visibleAvatar,
      published,
      locale,
      embedType,
      enableStreaming,
      theme,
      userId,
    } = values;
    const baseRoute =
      embedType === 'widget'
        ? Routes.ChatWidget
        : from === SharedFrom.Agent
          ? Routes.AgentShare
          : Routes.ChatShare;

    const src = new URL(`${location.origin}${baseRoute}`);
    src.searchParams.append('shared_id', token);
    src.searchParams.append('from', from);
    src.searchParams.append('auth', beta);

    if (published) {
      src.searchParams.append('release', 'true');
    }
    if (visibleAvatar) {
      src.searchParams.append('visible_avatar', '1');
    }
    if (locale) {
      src.searchParams.append('locale', locale);
    }
    if (embedType === 'widget') {
      src.searchParams.append('mode', 'master');
      src.searchParams.append('streaming', String(enableStreaming));
    }
    if (theme && embedType === 'fullscreen') {
      src.searchParams.append('theme', theme);
    }
    if (!isEmpty(trim(userId))) {
      src.searchParams.append('userId', userId!);
    }

    return src.toString();
  }, [beta, from, token, values]);

  const text = useMemo(() => {
    const iframeSrc = generateIframeSrc();
    const { embedType } = values;

    if (embedType === 'widget') {
      return `<iframe
  src="${iframeSrc}"
  style="position:fixed;bottom:0;right:0;width:100px;height:100px;border:none;background:transparent;z-index:9999"
  frameborder="0"
  allow="microphone;camera"
></iframe>
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
`;
    } else {
      return `<iframe
  src="${iframeSrc}"
  style="width: 100%; height: 100%; min-height: 600px"
  frameborder="0"
></iframe>
`;
    }
  }, [generateIframeSrc, values]);

  const handleOpenInNewTab = useCallback(() => {
    const iframeSrc = generateIframeSrc();
    window.open(iframeSrc, '_blank');
  }, [generateIframeSrc]);

  return (
    <Dialog open={visible} onOpenChange={hideModal}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('common.embedIntoSite')}</DialogTitle>
        </DialogHeader>

        <section className="w-full overflow-auto space-y-5 text-sm text-text-secondary">
          <Form {...form}>
            <form className="space-y-5">
              <FormField
                control={form.control}
                name="embedType"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('chat.embedType')}</FormLabel>
                    <FormControl>
                      <RadioGroup
                        onValueChange={field.onChange}
                        value={field.value}
                        className="flex flex-col space-y-2"
                      >
                        <div className="flex items-center space-x-2">
                          <RadioGroupItem value="fullscreen" id="fullscreen" />
                          <Label htmlFor="fullscreen" className="text-sm">
                            {t('chat.fullscreenChat')}
                          </Label>
                        </div>
                        <div className="flex items-center space-x-2">
                          <RadioGroupItem value="widget" id="widget" />
                          <Label htmlFor="widget" className="text-sm">
                            {t('chat.floatingWidget')}
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
                      <FormLabel>{t('chat.theme')}</FormLabel>
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
                              {t('chat.light')}
                            </Label>
                          </div>
                          <div className="flex items-center space-x-2">
                            <RadioGroupItem value={ThemeEnum.Dark} id="dark" />
                            <Label htmlFor="dark" className="text-sm">
                              {t('chat.dark')}
                            </Label>
                          </div>
                        </RadioGroup>
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              )}
              <SwitchFormField
                name="visibleAvatar"
                label={t('chat.avatarHidden')}
              ></SwitchFormField>
              {isAgent && (
                <SwitchFormField
                  name="published"
                  label={t('chat.published')}
                  tooltip={t('chat.publishedTooltip')}
                ></SwitchFormField>
              )}
              {values.embedType === 'widget' && (
                <SwitchFormField
                  name="enableStreaming"
                  label={t('chat.enableStreaming')}
                ></SwitchFormField>
              )}
              <RAGFlowFormItem name="locale" label={t('chat.locale')}>
                <SelectWithSearch options={languageOptions}></SelectWithSearch>
              </RAGFlowFormItem>
              <RAGFlowFormItem name="userId" label={t('flow.userId')}>
                <Input></Input>
              </RAGFlowFormItem>
            </form>
          </Form>
          <div>
            <span>{t('search.embedCode')}</span>
            <div>
              <SyntaxHighlighter
                className="max-h-[350px] overflow-auto scrollbar-auto"
                language="html"
                style={isDarkTheme ? oneDark : oneLight}
              >
                {text}
              </SyntaxHighlighter>
            </div>
          </div>
          <Button
            onClick={handleOpenInNewTab}
            className="w-full"
            variant="secondary"
          >
            <ExternalLink className="mr-2 h-4 w-4" />
            {t('common.openInNewTab')}
          </Button>
          <div className=" font-medium mt-4 mb-1">
            {t(isAgent ? 'flow' : 'chat', { keyPrefix: 'header' })}
            <span className="ml-1 inline-block">ID</span>
          </div>
          <div className="bg-bg-card rounded-lg flex items-center justify-between p-2">
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
            {t(`${isAgent ? 'flow' : 'chat'}.howUseId`)}
          </a>
        </section>
      </DialogContent>
    </Dialog>
  );
}

export default memo(EmbedDialog);
