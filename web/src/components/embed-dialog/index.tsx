import CopyToClipboard from '@/components/copy-to-clipboard';
import { SelectWithSearch } from '@/components/originui/select-with-search';
import { Button, ButtonLoading } from '@/components/ui/button';
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
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
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
  muteWidget: z.boolean(),
  theme: z.enum([ThemeEnum.Light, ThemeEnum.Dark]),
  userId: z.string().optional(),
  widgetTitle: z.string(),
  widgetSubtitle: z.string(),
  widgetFooterText: z.string(),
  widgetFooterLink: z.string(),
  widgetAccentColor: z.string(),
  widgetBackgroundColor: z.string(),
  widgetTextColor: z.string(),
  widgetHeaderTextColor: z.string(),
  widgetFooterTextColor: z.string(),
});

export type WidgetSettings = Pick<
  z.infer<typeof FormSchema>,
  | 'enableStreaming'
  | 'muteWidget'
  | 'widgetTitle'
  | 'widgetSubtitle'
  | 'widgetFooterText'
  | 'widgetFooterLink'
  | 'widgetAccentColor'
  | 'widgetBackgroundColor'
  | 'widgetTextColor'
  | 'widgetHeaderTextColor'
  | 'widgetFooterTextColor'
>;

export const defaultWidgetSettings: WidgetSettings = {
  enableStreaming: false,
  muteWidget: false,
  widgetTitle: '',
  widgetSubtitle: '',
  widgetFooterText: '',
  widgetFooterLink: '',
  widgetAccentColor: '#2563eb',
  widgetBackgroundColor: '#ffffff',
  widgetTextColor: '#111827',
  widgetHeaderTextColor: '#ffffff',
  widgetFooterTextColor: '#111827',
};

type IProps = IModalProps<any> & {
  token: string;
  from: SharedFrom;
  beta: string;
  isAgent: boolean;
  initialWidgetSettings?: Partial<WidgetSettings>;
  onSaveWidgetSettings?: (settings: WidgetSettings) => Promise<unknown>;
  savingWidgetSettings?: boolean;
};

const normalizeHexColor = (value: string | undefined, fallback: string) => {
  const normalizedValue = value?.trim() ?? '';
  return /^#([0-9a-f]{3}|[0-9a-f]{6})$/i.test(normalizedValue)
    ? normalizedValue
    : fallback;
};

/**
 * Builds the embed code preview and customization UI for shared chat and agent widgets.
 */
function EmbedDialog({
  hideModal,
  token = '',
  from,
  beta = '',
  isAgent,
  initialWidgetSettings,
  onSaveWidgetSettings,
  savingWidgetSettings,
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
      theme: ThemeEnum.Light,
      ...defaultWidgetSettings,
      ...initialWidgetSettings,
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
      muteWidget,
      theme,
      userId,
      widgetTitle,
      widgetSubtitle,
      widgetFooterText,
      widgetFooterLink,
      widgetAccentColor,
      widgetBackgroundColor,
      widgetTextColor,
      widgetHeaderTextColor,
      widgetFooterTextColor,
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
      src.searchParams.append('muted', String(muteWidget));
      if (!isEmpty(trim(widgetTitle))) {
        src.searchParams.append('widget_title', widgetTitle ?? '');
      }
      if (!isEmpty(trim(widgetSubtitle))) {
        src.searchParams.append('widget_subtitle', widgetSubtitle ?? '');
      }
      if (!isEmpty(trim(widgetFooterText))) {
        src.searchParams.append('widget_footer', widgetFooterText ?? '');
      }
      if (!isEmpty(trim(widgetFooterLink))) {
        src.searchParams.append('widget_footer_link', widgetFooterLink ?? '');
      }
      src.searchParams.append(
        'widget_accent_color',
        normalizeHexColor(widgetAccentColor, '#2563eb'),
      );
      src.searchParams.append(
        'widget_background_color',
        normalizeHexColor(widgetBackgroundColor, '#ffffff'),
      );
      src.searchParams.append(
        'widget_text_color',
        normalizeHexColor(widgetTextColor, '#111827'),
      );
      src.searchParams.append(
        'widget_header_text_color',
        normalizeHexColor(widgetHeaderTextColor, '#ffffff'),
      );
      src.searchParams.append(
        'widget_footer_text_color',
        normalizeHexColor(widgetFooterTextColor, '#111827'),
      );
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

  const handleSaveWidgetSettings = useCallback(async () => {
    if (!onSaveWidgetSettings) {
      return;
    }

    await onSaveWidgetSettings({
      enableStreaming: values.enableStreaming,
      muteWidget: values.muteWidget,
      widgetTitle: values.widgetTitle,
      widgetSubtitle: values.widgetSubtitle,
      widgetFooterText: values.widgetFooterText,
      widgetFooterLink: values.widgetFooterLink,
      widgetAccentColor: values.widgetAccentColor,
      widgetBackgroundColor: values.widgetBackgroundColor,
      widgetTextColor: values.widgetTextColor,
      widgetHeaderTextColor: values.widgetHeaderTextColor,
      widgetFooterTextColor: values.widgetFooterTextColor,
    });
  }, [onSaveWidgetSettings, values]);

  return (
    <Dialog open={visible} onOpenChange={hideModal}>
      <DialogContent className="sm:max-w-4xl">
        <DialogHeader>
          <DialogTitle>{t('common.embedIntoSite')}</DialogTitle>
        </DialogHeader>

        <section className="w-full overflow-auto space-y-5 text-sm text-text-secondary">
          <Form {...form}>
            <form className="space-y-5">
              <Tabs defaultValue="embed" className="w-full">
                <TabsList className="grid w-full grid-cols-2">
                  <TabsTrigger value="embed">Embed Setup</TabsTrigger>
                  <TabsTrigger value="widget">Widget Customization</TabsTrigger>
                </TabsList>
                <TabsContent value="embed" className="space-y-5">
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
                              <RadioGroupItem
                                value="fullscreen"
                                id="fullscreen"
                              />
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
                                <RadioGroupItem
                                  value={ThemeEnum.Dark}
                                  id="dark"
                                />
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
                  {values.embedType === 'widget' && (
                    <SwitchFormField
                      name="muteWidget"
                      label={t('chat.muteWidget')}
                    ></SwitchFormField>
                  )}
                  <RAGFlowFormItem name="locale" label={t('chat.locale')}>
                    <SelectWithSearch
                      options={languageOptions}
                    ></SelectWithSearch>
                  </RAGFlowFormItem>
                  {isAgent && (
                    <RAGFlowFormItem name="userId" label={t('flow.userId')}>
                      <Input></Input>
                    </RAGFlowFormItem>
                  )}
                </TabsContent>
                <TabsContent value="widget" className="space-y-5">
                  <div className="rounded-md border border-border p-3 text-xs text-text-secondary">
                    These settings apply to the floating widget embed.
                  </div>
                  <div className="grid gap-4 md:grid-cols-2">
                    <RAGFlowFormItem name="widgetTitle" label="Widget title">
                      <Input placeholder="Chat Support"></Input>
                    </RAGFlowFormItem>
                    <RAGFlowFormItem name="widgetSubtitle" label="Subtitle">
                      <Input placeholder="We typically reply instantly"></Input>
                    </RAGFlowFormItem>
                    <RAGFlowFormItem
                      name="widgetFooterText"
                      label="Footer text"
                    >
                      <Input placeholder="Powered by RAGFlow"></Input>
                    </RAGFlowFormItem>
                    <RAGFlowFormItem
                      name="widgetFooterLink"
                      label="Footer redirect link"
                    >
                      <Input placeholder="https://ragflow.io"></Input>
                    </RAGFlowFormItem>
                    <FormField
                      control={form.control}
                      name="widgetAccentColor"
                      render={({ field }) => (
                        <FormItem>
                          <FormLabel>Widget accent color</FormLabel>
                          <FormControl>
                            <div className="flex items-center gap-3">
                              <Input
                                type="color"
                                value={normalizeHexColor(
                                  field.value,
                                  '#2563eb',
                                )}
                                onChange={field.onChange}
                                className="h-10 w-16 p-1"
                              />
                              <Input {...field}></Input>
                            </div>
                          </FormControl>
                          <FormMessage />
                        </FormItem>
                      )}
                    />
                    <FormField
                      control={form.control}
                      name="widgetBackgroundColor"
                      render={({ field }) => (
                        <FormItem>
                          <FormLabel>Background color</FormLabel>
                          <FormControl>
                            <div className="flex items-center gap-3">
                              <Input
                                type="color"
                                value={normalizeHexColor(
                                  field.value,
                                  '#ffffff',
                                )}
                                onChange={field.onChange}
                                className="h-10 w-16 p-1"
                              />
                              <Input {...field}></Input>
                            </div>
                          </FormControl>
                          <FormMessage />
                        </FormItem>
                      )}
                    />
                    <FormField
                      control={form.control}
                      name="widgetTextColor"
                      render={({ field }) => (
                        <FormItem>
                          <FormLabel>Text color</FormLabel>
                          <FormControl>
                            <div className="flex items-center gap-3">
                              <Input
                                type="color"
                                value={normalizeHexColor(
                                  field.value,
                                  '#111827',
                                )}
                                onChange={field.onChange}
                                className="h-10 w-16 p-1"
                              />
                              <Input {...field}></Input>
                            </div>
                          </FormControl>
                          <FormMessage />
                        </FormItem>
                      )}
                    />
                    <FormField
                      control={form.control}
                      name="widgetHeaderTextColor"
                      render={({ field }) => (
                        <FormItem>
                          <FormLabel>Header text color</FormLabel>
                          <FormControl>
                            <div className="flex items-center gap-3">
                              <Input
                                type="color"
                                value={normalizeHexColor(
                                  field.value,
                                  '#ffffff',
                                )}
                                onChange={field.onChange}
                                className="h-10 w-16 p-1"
                              />
                              <Input {...field}></Input>
                            </div>
                          </FormControl>
                          <FormMessage />
                        </FormItem>
                      )}
                    />
                    <FormField
                      control={form.control}
                      name="widgetFooterTextColor"
                      render={({ field }) => (
                        <FormItem>
                          <FormLabel>Footer text color</FormLabel>
                          <FormControl>
                            <div className="flex items-center gap-3">
                              <Input
                                type="color"
                                value={normalizeHexColor(
                                  field.value,
                                  '#111827',
                                )}
                                onChange={field.onChange}
                                className="h-10 w-16 p-1"
                              />
                              <Input {...field}></Input>
                            </div>
                          </FormControl>
                          <FormMessage />
                        </FormItem>
                      )}
                    />
                  </div>
                </TabsContent>
              </Tabs>
            </form>
          </Form>
          <div>
            <span>{t('search.embedCode')}</span>
            <div className="relative">
              <CopyToClipboard
                text={text}
                className="absolute right-3 top-3 z-10 border border-border bg-background/90 backdrop-blur-sm"
              />
              <SyntaxHighlighter
                className="max-h-[350px] overflow-auto scrollbar-auto pr-14"
                language="html"
                style={isDarkTheme ? oneDark : oneLight}
              >
                {text}
              </SyntaxHighlighter>
            </div>
          </div>
          <div className="flex gap-3">
            {isAgent && onSaveWidgetSettings && (
              <ButtonLoading
                onClick={handleSaveWidgetSettings}
                loading={savingWidgetSettings}
                className="flex-1"
                variant="secondary"
              >
                {t('flow.save')} widget settings
              </ButtonLoading>
            )}
            <Button
              onClick={handleOpenInNewTab}
              className={isAgent && onSaveWidgetSettings ? 'flex-1' : 'w-full'}
              variant="secondary"
            >
              <ExternalLink className="mr-2 h-4 w-4" />
              {t('common.openInNewTab')}
            </Button>
          </div>
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
