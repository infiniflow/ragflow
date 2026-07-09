import { FormFieldConfig, FormFieldType } from '@/components/dynamic-form';
import SvgIcon from '@/components/svg-icon';
import { TFunction } from 'i18next';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IChatChannelInfoMap } from '../interface';

export enum ChatChannelKey {
  CLICKCLACK = 'clickclack',
  DISCORD = 'discord',
  DINGTALK = 'dingtalk',
  FEISHU = 'feishu',
  GOOGLECHAT = 'googlechat',
  IRC = 'irc',
  LINE = 'line',
  MATRIX = 'matrix',
  MATTERMOST = 'mattermost',
  MSTEAMS = 'msteams',
  NEXTCLOUD_TALK = 'nextcloud_talk',
  NOSTR = 'nostr',
  QQBOT = 'qqbot',
  SLACK = 'slack',
  SYNOLOGY_CHAT = 'synology_chat',
  TELEGRAM = 'telegram',
  TLON = 'tlon',
  TWITCH = 'twitch',
  WECOM = 'wecom',
  WHATSAPP = 'whatsapp',
  YUANBAO = 'yuanbao',
  ZALO = 'zalo',
  ZALOUSER = 'zalouser',
}

// Channels whose logo already exists in the shared data-source asset set —
// reuse it instead of duplicating an icon under chat-channel/.
const SHARED_DATA_SOURCE_ICON: Partial<Record<ChatChannelKey, string>> = {
  [ChatChannelKey.DISCORD]: 'data-source/discord',
  [ChatChannelKey.SLACK]: 'data-source/slack',
  [ChatChannelKey.MSTEAMS]: 'data-source/teams',
};

const channelIcon = (key: ChatChannelKey) => (
  <SvgIcon
    name={SHARED_DATA_SOURCE_ICON[key] ?? `chat-channel/${key}`}
    width={38}
  />
);

const CHANNEL_NAMES: Record<ChatChannelKey, string> = {
  [ChatChannelKey.CLICKCLACK]: 'ClickClack',
  [ChatChannelKey.DISCORD]: 'Discord',
  [ChatChannelKey.DINGTALK]: 'DingTalk',
  [ChatChannelKey.FEISHU]: 'Feishu / Lark',
  [ChatChannelKey.GOOGLECHAT]: 'Google Chat',
  [ChatChannelKey.IRC]: 'IRC',
  [ChatChannelKey.LINE]: 'LINE',
  [ChatChannelKey.MATRIX]: 'Matrix',
  [ChatChannelKey.MATTERMOST]: 'Mattermost',
  [ChatChannelKey.MSTEAMS]: 'Microsoft Teams',
  [ChatChannelKey.NEXTCLOUD_TALK]: 'Nextcloud Talk',
  [ChatChannelKey.NOSTR]: 'Nostr',
  [ChatChannelKey.QQBOT]: 'QQ Bot',
  [ChatChannelKey.SLACK]: 'Slack',
  [ChatChannelKey.SYNOLOGY_CHAT]: 'Synology Chat',
  [ChatChannelKey.TELEGRAM]: 'Telegram',
  [ChatChannelKey.TLON]: 'Tlon (Urbit)',
  [ChatChannelKey.TWITCH]: 'Twitch',
  [ChatChannelKey.WECOM]: 'WeCom',
  [ChatChannelKey.WHATSAPP]: 'WhatsApp',
  [ChatChannelKey.YUANBAO]: 'Yuanbao',
  [ChatChannelKey.ZALO]: 'Zalo',
  [ChatChannelKey.ZALOUSER]: 'Zalo (Personal)',
};

export const generateChatChannelInfo = (t: TFunction): IChatChannelInfoMap =>
  Object.values(ChatChannelKey).reduce((acc, key) => {
    acc[key] = {
      name: CHANNEL_NAMES[key],
      description: t(`setting.chatChannelDesc.${key}`),
      icon: channelIcon(key),
    };
    return acc;
  }, {} as IChatChannelInfoMap);

export const useChatChannelInfo = () => {
  const { t } = useTranslation();
  const [chatChannelInfo, setChatChannelInfo] = useState<IChatChannelInfoMap>(
    generateChatChannelInfo(t) as IChatChannelInfoMap,
  );
  useEffect(() => {
    setChatChannelInfo(generateChatChannelInfo(t) as IChatChannelInfoMap);
  }, [t]);
  return { chatChannelInfo };
};

export const getChatChannelRuntimeStatusClass = (status?: string) => {
  const normalized = (status || '').toLowerCase();
  if (normalized === 'connected') {
    return 'bg-state-success/10 text-state-success border-state-success/20';
  }
  if (
    normalized === 'connecting' ||
    normalized === 'reconnecting' ||
    normalized === 'qr'
  ) {
    return 'bg-state-warning/10 text-state-warning border-state-warning/20';
  }
  if (normalized === 'waiting') {
    return 'bg-state-warning/10 text-state-warning border-state-warning/20';
  }
  if (
    normalized === 'error' ||
    normalized === 'disconnected' ||
    normalized === 'stopped'
  ) {
    return 'bg-state-error/10 text-state-error border-state-error/20';
  }
  return 'bg-gray-500/10 text-text-secondary border-border-button';
};

export const getChatChannelRuntimeStatusText = (status?: string) => {
  const normalized = (status || '').toLowerCase();
  if (normalized === 'connected') {
    return 'Connected';
  }
  if (normalized === 'connecting') {
    return 'Connecting...';
  }
  if (normalized === 'reconnecting') {
    return 'Reconnecting...';
  }
  if (normalized === 'qr') {
    return 'Scan the QR code below';
  }
  if (normalized === 'waiting') {
    return 'Waiting for the channel to start';
  }
  if (normalized === 'error') {
    return 'Runtime error';
  }
  if (normalized === 'disconnected') {
    return 'Disconnected';
  }
  if (normalized === 'stopped') {
    return 'Stopped';
  }
  return 'Waiting for runtime...';
};

const isPlainObject = (value: unknown): value is Record<string, any> =>
  typeof value === 'object' && value !== null && !Array.isArray(value);

export const mergeChatChannelFormValues = (
  ...values: Array<Record<string, any> | undefined>
): Record<string, any> =>
  values.reduce<Record<string, any>>((result, current) => {
    if (!current) {
      return result;
    }
    const next = { ...result };
    Object.entries(current).forEach(([key, value]) => {
      if (isPlainObject(value) && isPlainObject(next[key])) {
        next[key] = mergeChatChannelFormValues(next[key], value);
      } else {
        next[key] = value;
      }
    });
    return next;
  }, {});

export const ChatChannelFormBaseFields: FormFieldConfig[] = [
  {
    name: 'id',
    type: FormFieldType.Text,
    required: false,
    hidden: true,
  },
  {
    label: 'Name',
    name: 'name',
    type: FormFieldType.Text,
    required: true,
  },
  {
    name: 'channel',
    type: FormFieldType.Text,
    required: true,
    hidden: true,
  },
] as FormFieldConfig[];

// Per-channel credential fields. Credentials are stored under
// `config.credential.*` to match the backend JSON shape.
export const ChatChannelFormFields: Record<ChatChannelKey, FormFieldConfig[]> =
  {
    [ChatChannelKey.CLICKCLACK]: [
      {
        label: 'Bot Token',
        name: 'config.credential.token',
        type: FormFieldType.Password,
        required: true,
      },
    ],
    [ChatChannelKey.DISCORD]: [
      {
        label: 'Bot Token',
        name: 'config.credential.token',
        type: FormFieldType.Password,
        required: true,
      },
      {
        label: 'Application ID',
        name: 'config.credential.application_id',
        type: FormFieldType.Text,
        required: false,
        placeholder: '1234567890',
      },
    ],
    [ChatChannelKey.DINGTALK]: [
      {
        label: 'Client ID',
        name: 'config.credential.client_id',
        type: FormFieldType.Text,
        required: true,
        placeholder: 'dingxxxxxxxxxxxx',
      },
      {
        label: 'Client Secret',
        name: 'config.credential.client_secret',
        type: FormFieldType.Password,
        required: true,
      },
    ],
    [ChatChannelKey.FEISHU]: [
      {
        label: 'App ID',
        name: 'config.credential.app_id',
        type: FormFieldType.Text,
        required: true,
        placeholder: 'cli_xxxxxxxxxxxxxxxx',
      },
      {
        label: 'App Secret',
        name: 'config.credential.app_secret',
        type: FormFieldType.Password,
        required: true,
      },
      {
        label: 'Domain',
        name: 'config.credential.domain',
        type: FormFieldType.Select,
        required: true,
        defaultValue: 'feishu',
        options: [
          { label: 'Feishu (mainland)', value: 'feishu' },
          { label: 'Lark (international)', value: 'lark' },
        ],
      },
    ],
    [ChatChannelKey.GOOGLECHAT]: [
      {
        label: 'Auth Mode',
        name: 'config.auth_mode',
        type: FormFieldType.Select,
        required: true,
        defaultValue: 'webhook_url',
        options: [
          { label: 'Webhook URL', value: 'webhook_url' },
          { label: 'Service Account JSON', value: 'service_account' },
          { label: 'Service Account File', value: 'service_account_file' },
        ],
      },
      {
        label: 'Webhook URL',
        name: 'config.credential.webhook_url',
        type: FormFieldType.Text,
        required: false,
        placeholder:
          'https://chat.googleapis.com/v1/spaces/.../messages?key=...&token=...',
        shouldRender: (values: any) =>
          values?.config?.auth_mode === 'webhook_url',
        customValidate: (val: string, values: any) =>
          values?.config?.auth_mode === 'webhook_url' && !(val ?? '').trim()
            ? 'Webhook URL is required for this auth mode'
            : true,
      },
      {
        label: 'Service Account JSON',
        name: 'config.credential.service_account',
        type: FormFieldType.Textarea,
        required: false,
        placeholder: '{ "type": "service_account", ... }',
        shouldRender: (values: any) =>
          values?.config?.auth_mode === 'service_account',
        customValidate: (val: string, values: any) =>
          values?.config?.auth_mode === 'service_account' && !(val ?? '').trim()
            ? 'Service account JSON is required for this auth mode'
            : true,
      },
      {
        label: 'Service Account File Path',
        name: 'config.credential.service_account_file',
        type: FormFieldType.Text,
        required: false,
        placeholder: '/path/to/sa.json',
        shouldRender: (values: any) =>
          values?.config?.auth_mode === 'service_account_file',
        customValidate: (val: string, values: any) =>
          values?.config?.auth_mode === 'service_account_file' &&
          !(val ?? '').trim()
            ? 'Service account file path is required for this auth mode'
            : true,
      },
    ],
    [ChatChannelKey.IRC]: [
      {
        label: 'Host',
        name: 'config.credential.host',
        type: FormFieldType.Text,
        required: true,
        placeholder: 'irc.libera.chat',
      },
      {
        label: 'Port',
        name: 'config.credential.port',
        type: FormFieldType.Number,
        required: true,
        placeholder: '6697',
      },
      {
        label: 'TLS',
        name: 'config.credential.tls',
        type: FormFieldType.Switch,
        required: false,
        defaultValue: true,
      },
      {
        label: 'Nick',
        name: 'config.credential.nick',
        type: FormFieldType.Text,
        required: true,
        placeholder: 'my-bot',
      },
      {
        label: 'Username',
        name: 'config.credential.username',
        type: FormFieldType.Text,
        required: false,
        placeholder: 'Defaults to nick',
      },
      {
        label: 'Server Password',
        name: 'config.credential.password',
        type: FormFieldType.Password,
        required: false,
      },
      {
        label: 'NickServ Account',
        name: 'config.credential.nickserv.account',
        type: FormFieldType.Text,
        required: false,
      },
      {
        label: 'NickServ Password',
        name: 'config.credential.nickserv.password',
        type: FormFieldType.Password,
        required: false,
      },
    ],
    [ChatChannelKey.LINE]: [
      {
        label: 'Channel Secret',
        name: 'config.credential.channel_secret',
        type: FormFieldType.Password,
        required: true,
      },
      {
        label: 'Channel Access Token',
        name: 'config.credential.channel_access_token',
        type: FormFieldType.Password,
        required: true,
      },
    ],
    [ChatChannelKey.MATRIX]: [
      {
        label: 'Homeserver',
        name: 'config.credential.homeserver',
        type: FormFieldType.Text,
        required: true,
        placeholder: 'https://matrix.example.org',
      },
      {
        label: 'User ID',
        name: 'config.credential.user_id',
        type: FormFieldType.Text,
        required: true,
        placeholder: '@bot:example.org',
      },
      {
        label: 'Access Token',
        name: 'config.credential.access_token',
        type: FormFieldType.Password,
        required: false,
      },
      {
        label: 'Password',
        name: 'config.credential.password',
        type: FormFieldType.Password,
        required: false,
      },
      {
        label: 'Device ID',
        name: 'config.credential.device_id',
        type: FormFieldType.Text,
        required: false,
        placeholder: 'OPENCLAW_BOT',
      },
    ],
    [ChatChannelKey.MATTERMOST]: [
      {
        label: 'Base URL',
        name: 'config.credential.base_url',
        type: FormFieldType.Text,
        required: true,
        placeholder: 'https://mattermost.example.com',
      },
      {
        label: 'Bot Token',
        name: 'config.credential.bot_token',
        type: FormFieldType.Password,
        required: true,
      },
    ],
    [ChatChannelKey.MSTEAMS]: [
      {
        label: 'App ID',
        name: 'config.credential.app_id',
        type: FormFieldType.Text,
        required: true,
        placeholder: '00000000-0000-0000-0000-000000000000',
      },
      {
        label: 'App Password',
        name: 'config.credential.app_password',
        type: FormFieldType.Password,
        required: true,
      },
      {
        label: 'Tenant ID',
        name: 'config.credential.tenant_id',
        type: FormFieldType.Text,
        required: true,
        placeholder: '00000000-0000-0000-0000-000000000000',
      },
    ],
    [ChatChannelKey.NEXTCLOUD_TALK]: [
      {
        label: 'Base URL',
        name: 'config.credential.base_url',
        type: FormFieldType.Text,
        required: true,
        placeholder: 'https://nextcloud.example.com',
      },
      {
        label: 'Bot Secret',
        name: 'config.credential.bot_secret',
        type: FormFieldType.Password,
        required: true,
      },
    ],
    [ChatChannelKey.NOSTR]: [
      {
        label: 'Private Key',
        name: 'config.credential.private_key',
        type: FormFieldType.Password,
        required: true,
        placeholder: 'nsec1xxxx or hex',
      },
      {
        label: 'Relays',
        name: 'config.credential.relays',
        type: FormFieldType.Tag,
        required: false,
        placeholder: 'wss://relay.damus.io',
      },
    ],
    [ChatChannelKey.QQBOT]: [
      {
        label: 'App ID',
        name: 'config.credential.app_id',
        type: FormFieldType.Text,
        required: true,
        placeholder: '102000000',
      },
      {
        label: 'Client Secret',
        name: 'config.credential.client_secret',
        type: FormFieldType.Password,
        required: true,
      },
      {
        label: 'Base URL',
        name: 'config.credential.base_url',
        type: FormFieldType.Text,
        required: false,
        placeholder: 'https://api.sgroup.qq.com',
      },
    ],
    [ChatChannelKey.SLACK]: [
      {
        label: 'Bot Token',
        name: 'config.credential.bot_token',
        type: FormFieldType.Password,
        required: true,
        placeholder: 'xoxb-xxxx',
      },
      {
        label: 'App Token',
        name: 'config.credential.app_token',
        type: FormFieldType.Password,
        required: false,
        placeholder: 'xapp-xxxx (socket mode)',
      },
      {
        label: 'Signing Secret',
        name: 'config.credential.signing_secret',
        type: FormFieldType.Password,
        required: false,
      },
      {
        label: 'User Token',
        name: 'config.credential.user_token',
        type: FormFieldType.Password,
        required: false,
        placeholder: 'xoxp-xxxx',
      },
    ],
    [ChatChannelKey.SYNOLOGY_CHAT]: [
      {
        label: 'Token',
        name: 'config.credential.token',
        type: FormFieldType.Password,
        required: true,
      },
      {
        label: 'Incoming URL',
        name: 'config.credential.incoming_url',
        type: FormFieldType.Text,
        required: true,
        placeholder: 'https://nas.example.com/webapi/entry.cgi?...',
      },
    ],
    [ChatChannelKey.TELEGRAM]: [
      {
        label: 'Bot Token',
        name: 'config.credential.token',
        type: FormFieldType.Password,
        required: true,
        placeholder: 'From @BotFather',
      },
    ],
    [ChatChannelKey.TLON]: [
      {
        label: 'Ship',
        name: 'config.credential.ship',
        type: FormFieldType.Text,
        required: true,
        placeholder: '~sampel-palnet',
      },
      {
        label: 'URL',
        name: 'config.credential.url',
        type: FormFieldType.Text,
        required: true,
        placeholder: 'https://sampel-palnet.tlon.network',
      },
      {
        label: 'Code',
        name: 'config.credential.code',
        type: FormFieldType.Password,
        required: true,
      },
    ],
    [ChatChannelKey.TWITCH]: [
      {
        label: 'Username',
        name: 'config.credential.username',
        type: FormFieldType.Text,
        required: true,
        placeholder: 'my-bot',
      },
      {
        label: 'Access Token',
        name: 'config.credential.access_token',
        type: FormFieldType.Password,
        required: true,
        placeholder: 'oauth:xxxx',
      },
      {
        label: 'Client ID',
        name: 'config.credential.client_id',
        type: FormFieldType.Text,
        required: true,
      },
      {
        label: 'Channel',
        name: 'config.credential.channel',
        type: FormFieldType.Text,
        required: true,
        placeholder: 'target-channel-name',
      },
    ],
    [ChatChannelKey.WECOM]: [
      {
        label: 'Connection Type',
        name: 'config.credential.connection_type',
        type: FormFieldType.Select,
        required: true,
        defaultValue: 'webhook',
        options: [
          { label: 'Webhook', value: 'webhook' },
          { label: 'WebSocket', value: 'websocket' },
        ],
      },
      {
        label: 'Bot ID',
        name: 'config.credential.bot_id',
        type: FormFieldType.Text,
        required: true,
        placeholder: 'AIBOTID',
        shouldRender: (values: any) =>
          values?.config?.credential?.connection_type === 'websocket',
      },
      {
        label: 'Secret',
        name: 'config.credential.secret',
        type: FormFieldType.Password,
        required: true,
        placeholder: 'App Secret / Long-connection Secret',
      },
      {
        label: 'Corp ID',
        name: 'config.credential.corp_id',
        type: FormFieldType.Text,
        required: true,
        placeholder: 'ww1234567890abcdef',
        shouldRender: (values: any) =>
          values?.config?.credential?.connection_type !== 'websocket',
      },
      {
        label: 'Agent ID',
        name: 'config.credential.agent_id',
        type: FormFieldType.Number,
        required: true,
        placeholder: '1000001',
        shouldRender: (values: any) =>
          values?.config?.credential?.connection_type !== 'websocket',
      },
      {
        label: 'Token',
        name: 'config.credential.token',
        type: FormFieldType.Password,
        required: true,
        shouldRender: (values: any) =>
          values?.config?.credential?.connection_type !== 'websocket',
      },
      {
        label: 'AES Key',
        name: 'config.credential.aes_key',
        type: FormFieldType.Password,
        required: true,
        placeholder: '43 chars',
        shouldRender: (values: any) =>
          values?.config?.credential?.connection_type !== 'websocket',
      },
    ],
    [ChatChannelKey.WHATSAPP]: [],
    [ChatChannelKey.YUANBAO]: [
      {
        label: 'App Key',
        name: 'config.credential.app_key',
        type: FormFieldType.Text,
        required: true,
      },
      {
        label: 'App Secret',
        name: 'config.credential.app_secret',
        type: FormFieldType.Password,
        required: true,
      },
      {
        label: 'Token',
        name: 'config.credential.token',
        type: FormFieldType.Password,
        required: false,
        placeholder: 'Optional pre-signed token',
      },
    ],
    [ChatChannelKey.ZALO]: [
      {
        label: 'Bot Token',
        name: 'config.credential.bot_token',
        type: FormFieldType.Password,
        required: true,
      },
    ],
    [ChatChannelKey.ZALOUSER]: [
      {
        label: 'Cookies (JSON)',
        name: 'config.credential.cookies',
        type: FormFieldType.Textarea,
        required: true,
        placeholder: '{ ... }',
      },
      {
        label: 'IMEI',
        name: 'config.credential.imei',
        type: FormFieldType.Text,
        required: true,
      },
      {
        label: 'User Agent',
        name: 'config.credential.user_agent',
        type: FormFieldType.Text,
        required: true,
        placeholder: 'Mozilla/5.0 ...',
      },
    ],
  };

export const ChatChannelFormDefaultValues: Record<
  ChatChannelKey,
  Record<string, any>
> = Object.values(ChatChannelKey).reduce(
  (acc, key) => {
    acc[key] = {
      name: '',
      channel: key,
      config: { credential: {} },
    };
    return acc;
  },
  {} as Record<ChatChannelKey, Record<string, any>>,
);

// googlechat carries a non-credential discriminator (auth_mode).
ChatChannelFormDefaultValues[ChatChannelKey.GOOGLECHAT].config.auth_mode =
  'webhook_url';
ChatChannelFormDefaultValues[
  ChatChannelKey.WECOM
].config.credential.connection_type = 'webhook';
ChatChannelFormDefaultValues[ChatChannelKey.FEISHU].config.credential.domain =
  'feishu';
export const getChatChannelFields = (
  key?: ChatChannelKey,
): FormFieldConfig[] => {
  if (!key) {
    return [];
  }
  return [...ChatChannelFormBaseFields, ...(ChatChannelFormFields[key] || [])];
};
