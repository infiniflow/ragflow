'use client';

import { AvatarNameDescription } from '@/components/avatar-name-description';
import { KnowledgeBaseFormField } from '@/components/knowledge-base-item';
import { FailoverModelsField } from '@/components/llm-setting-items/failover-models-field';
import { LlmSettingFieldItems } from '@/components/llm-setting-items/next';
import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { Textarea } from '@/components/ui/textarea';
import { useFetchChat } from '@/hooks/use-chat-request';
import { useTranslate } from '@/hooks/common-hooks';
import { prefixName } from '@/utils/form';
import { getDirAttribute } from '@/utils/text-direction';
import { useFormContext, useWatch } from 'react-hook-form';

interface ChatBasicSettingProps {
  prefix?: string;
  option?: Record<string, any>;
  hideName?: boolean;
  collapseOpen?: boolean;
  onCollapseOpenChange?: (open: boolean) => void;
}

export default function ChatBasicSetting({
  prefix = '',
  hideName = false,
  collapseOpen,
  onCollapseOpenChange,
}: ChatBasicSettingProps) {
  const { t } = useTranslate('chat');
  const form = useFormContext();

  const prologueValue = useWatch({
    control: form.control,
    name: prefixName(prefix, 'prompt_config.prologue'),
  });

  const llmSettingPrefix = prefixName(prefix, 'llm_setting');

  // The failover list heads the chain with the dialog's own model, so the
  // editor needs that id and the tenant that owns the model list.
  const { data: chatData } = useFetchChat();
  const llmIdValue = useWatch({
    control: form.control,
    name: prefixName(prefix, 'llm_id'),
  });
  const ownerTenantId = chatData?.tenant_id;

  return (
    <div className="space-y-8">
      {hideName || (
        <AvatarNameDescription
          avatarField={prefixName(prefix, 'icon')}
          nameField={prefixName(prefix, 'name')}
          descriptionField={prefixName(prefix, 'description')}
        />
      )}
      <LlmSettingFieldItems
        prefix={llmSettingPrefix}
        llmId={prefixName(prefix, 'llm_id')}
        showCollapse
        collapseOpen={collapseOpen}
        onCollapseOpenChange={onCollapseOpenChange}
      ></LlmSettingFieldItems>

      <FailoverModelsField
        name={prefixName(prefix, 'llm_setting.failover_llm_ids')}
        ownerTenantId={ownerTenantId}
        primaryLlmId={llmIdValue}
      />

      <FormField
        control={form.control}
        name={prefixName(prefix, 'prompt_config.prologue')}
        render={({ field }) => (
          <FormItem>
            <FormLabel tooltip={t('setAnOpenerTip')}>
              {t('setAnOpener')}
            </FormLabel>
            <FormControl>
              <Textarea
                {...field}
                dir={getDirAttribute(prologueValue || '')}
              ></Textarea>
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />
      <KnowledgeBaseFormField
        name={prefixName(prefix, 'dataset_ids')}
      ></KnowledgeBaseFormField>
    </div>
  );
}
