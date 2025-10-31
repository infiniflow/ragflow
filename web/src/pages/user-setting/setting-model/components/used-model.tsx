import { LlmItem, useSelectLlmList } from '@/hooks/llm-hooks';
import { ModelProviderCard } from './modal-card';

export const UsedModel = ({
  handleAddModel,
  handleEditModel,
}: {
  handleAddModel: (factory: string) => void;
  handleEditModel: (model: any, factory: LlmItem) => void;
}) => {
  const { factoryList, myLlmList: llmList, loading } = useSelectLlmList();
  return (
    <div className="flex flex-col w-full gap-4 mb-4">
      <div className="text-text-primary text-2xl mb-2 mt-4">Added models</div>
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
