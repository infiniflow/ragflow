import { Checkbox } from '@/components/ui/checkbox';
import { useTranslate } from '@/hooks/common-hooks';
import { IProviderModelItem } from '@/interfaces/request/llm';
import { useMemo } from 'react';

interface UseListModelsOptionsParams {
  models: IProviderModelItem[];
  selectedModelItems: IProviderModelItem[];
  allSelected: boolean;
  handleSelectModel: (model: IProviderModelItem) => void;
  handleToggleAll: () => void;
}

/**
 * Build ToggleList options from the fetched model list. The first item is
 * a sentinel "All models" row that toggles the full selection.
 *
 * Why the Checkbox uses `onClick` (not `onCheckedChange`):
 * Radix Checkbox calls `event.stopPropagation()` internally on its onClick
 * when the Checkbox lives inside a form, so the row's `onClick` (attached
 * by ToggleList) never fires when the user clicks the Checkbox itself.
 * To make the Checkbox click toggle selection, we handle it in our own
 * `onClick` and re-stop propagation to (a) prevent the row's onClick from
 * double-firing and (b) make Radix's CheckboxBubbleInput dispatch a
 * non-bubbling synthetic click on its hidden form input — without this,
 * the dispatched click would bubble back to the row and re-trigger the
 * toggle, causing "Maximum update depth exceeded".
 */
export const useListModelsOptions = ({
  models,
  selectedModelItems,
  allSelected,
  handleSelectModel,
  handleToggleAll,
}: UseListModelsOptionsParams) => {
  const { t } = useTranslate('setting');

  return useMemo(() => {
    const allOption = {
      value: null as string | null,
      label: (
        <div className="flex justify-between items-center gap-2 w-full">
          <div className="flex-1 min-w-0 flex gap-1 items-center">
            <div className="font-medium truncate">{t('allModels')}</div>
          </div>
          <Checkbox
            checked={allSelected}
            onClick={(e) => {
              e.stopPropagation();
              handleToggleAll();
            }}
          />
        </div>
      ),
      onClick: () => handleToggleAll(),
    };

    const modelOptions = models.map((m) => {
      const checked = selectedModelItems.some((s) => s.name === m.name);
      return {
        value: m.name,
        label: (
          <div className="flex justify-between items-center gap-2 w-full">
            <div className="flex-1 min-w-0 flex gap-1 items-center">
              <div className="font-medium truncate">{m.name}</div>
              {m.model_types &&
                m.model_types.map((type) => {
                  return (
                    <div
                      key={type}
                      className="text-xs text-text-secondary truncate bg-bg-card rounded-md px-2 py-1"
                    >
                      {type}
                    </div>
                  );
                })}
            </div>
            <Checkbox
              checked={checked}
              onClick={(e) => {
                e.stopPropagation();
                handleSelectModel(m);
              }}
            />
          </div>
        ),
        onClick: () => handleSelectModel(m),
      };
    });
    if (modelOptions?.length) {
      return [allOption, ...modelOptions];
    } else {
      return [];
    }
  }, [
    models,
    selectedModelItems,
    handleSelectModel,
    allSelected,
    handleToggleAll,
    t,
  ]);
};
