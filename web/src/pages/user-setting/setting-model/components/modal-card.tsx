// src/components/ModelProviderCard.tsx
import { LlmIcon } from '@/components/svg-icon';
import { Button } from '@/components/ui/button';
import { Switch } from '@/components/ui/switch';
import { useSetModalState, useTranslate } from '@/hooks/common-hooks';
import { LlmItem } from '@/hooks/llm-hooks';
import { getRealModelName } from '@/utils/llm-util';
import { EditOutlined, SettingOutlined } from '@ant-design/icons';
import { ChevronsDown, ChevronsUp, Trash2 } from 'lucide-react';
import { FC } from 'react';
import { isLocalLlmFactory } from '../../utils';
import { useHandleDeleteFactory, useHandleEnableLlm } from '../hooks';

interface IModelCardProps {
  item: LlmItem;
  clickApiKey: (llmFactory: string) => void;
  handleEditModel: (model: any, factory: LlmItem) => void;
}

type TagType =
  | 'LLM'
  | 'TEXT EMBEDDING'
  | 'TEXT RE-RANK'
  | 'TTS'
  | 'SPEECH2TEXT'
  | 'IMAGE2TEXT'
  | 'MODERATION';

const sortTags = (tags: string) => {
  const orderMap: Record<TagType, number> = {
    LLM: 1,
    'TEXT EMBEDDING': 2,
    'TEXT RE-RANK': 3,
    TTS: 4,
    SPEECH2TEXT: 5,
    IMAGE2TEXT: 6,
    MODERATION: 7,
  };

  return tags
    .split(',')
    .map((tag) => tag.trim())
    .sort(
      (a, b) =>
        (orderMap[a as TagType] || 999) - (orderMap[b as TagType] || 999),
    );
};

export const ModelProviderCard: FC<IModelCardProps> = ({
  item,
  clickApiKey,
  handleEditModel,
}) => {
  const { visible, switchVisible } = useSetModalState();
  const { t } = useTranslate('setting');
  const { handleEnableLlm } = useHandleEnableLlm(item.name);
  const { handleDeleteFactory } = useHandleDeleteFactory(item.name);

  const handleApiKeyClick = () => {
    clickApiKey(item.name);
  };

  const handleShowMoreClick = () => {
    switchVisible();
  };

  return (
    <div className={`w-full rounded-lg border border-border-button`}>
      {/* Header */}
      <div className="flex h-16  items-center justify-between p-4 cursor-pointer transition-colors text-text-secondary">
        <div className="flex items-center space-x-3">
          <LlmIcon name={item.name} />
          <div>
            <div className="font-medium text-xl">{item.name}</div>
          </div>
        </div>

        <div className="flex items-center space-x-2">
          <Button
            variant={'ghost'}
            onClick={(e) => {
              e.stopPropagation();
              handleApiKeyClick();
            }}
            className="px-3 py-1 text-sm    rounded-md transition-colors flex items-center space-x-1 border border-border-default"
          >
            <SettingOutlined />
            <span>
              {isLocalLlmFactory(item.name) ? t('addTheModel') : 'API-Key'}
            </span>
          </Button>

          <Button
            variant={'ghost'}
            onClick={(e) => {
              e.stopPropagation();
              handleShowMoreClick();
            }}
            className="px-3 py-1 text-sm   rounded-md transition-colors flex items-center space-x-1 border border-border-default"
          >
            <span>{visible ? t('hideModels') : t('showMoreModels')}</span>
            {!visible ? <ChevronsDown /> : <ChevronsUp />}
          </Button>

          <Button
            variant={'ghost'}
            onClick={(e) => {
              e.stopPropagation();
              handleDeleteFactory();
            }}
            className="  hover:text-state-error hover:bg-state-error-5 transition-colors border border-border-default"
          >
            <Trash2 />
          </Button>
        </div>
      </div>

      {/* Content */}
      {visible && (
        <div className="">
          <div className="px-4 flex flex-wrap gap-1 mt-1">
            {sortTags(item.tags).map((tag, index) => (
              <span
                key={index}
                className="px-2 py-1 text-xs bg-bg-card text-text-secondary rounded-md"
              >
                {tag}
              </span>
            ))}
          </div>
          <div className="m-4 bg-bg-card rounded-lg max-h-96 overflow-auto scrollbar-auto">
            <div className="">
              {item.llm.map((model) => (
                <div
                  key={model.name}
                  className="flex items-center border-b-[0.5px] border-border-button justify-between p-3 hover:bg-bg-card transition-colors"
                >
                  <div className="flex items-center space-x-3">
                    <span className="font-medium">
                      {getRealModelName(model.name)}
                    </span>
                    <span className="px-2 py-1 text-xs bg-bg-card text-text-secondary rounded-md">
                      {model.type}
                    </span>
                  </div>

                  <div className="flex items-center space-x-2">
                    {isLocalLlmFactory(item.name) && (
                      <Button
                        variant={'secondary'}
                        onClick={() => handleEditModel(model, item)}
                        className="p-1 text-text-primary transition-colors"
                      >
                        <EditOutlined />
                      </Button>
                    )}
                    <Switch
                      checked={model.status === '1'}
                      onCheckedChange={(value) => {
                        handleEnableLlm(model.name, value);
                      }}
                    />
                  </div>
                </div>
              ))}
            </div>
          </div>
        </div>
      )}
    </div>
  );
};
