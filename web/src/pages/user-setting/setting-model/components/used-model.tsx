import { LlmItem, useSelectLlmList } from '@/hooks/use-llm-request';
import { t } from 'i18next';
import { ModelProviderCard } from './modal-card';

export const UsedModel = ({
  handleAddModel,
  handleEditModel,
}: {
  handleAddModel: (factory: string) => void;
  handleEditModel: (model: any, factory: LlmItem) => void;
}) => {
  const { myLlmList: llmList } = useSelectLlmList();
  return (
    <div className="flex flex-col w-full gap-5 mb-4">
      <div className="text-text-primary text-2xl font-medium mb-2 mt-4">
        {t('setting.addedModels')}
      </div>
      {llmList.map((llm) => {
        return (
          <ModelProviderCard
            key={llm.name}
            item={llm}
            clickApiKey={handleAddModel}
            handleEditModel={handleEditModel}
          />
        );
      })}
    </div>
  );
};
