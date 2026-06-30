import { ModelTreeSelect, ModelTypeMap } from '@/components/model-tree-select';
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip';
import { FieldToModelType } from '@/constants/llm';
import { useTranslate } from '@/hooks/common-hooks';
import {
  useFetchDefaultModelDictionary,
  useSetDefaultModel,
} from '@/hooks/use-llm-request';
import { parseModelValue } from '@/utils/llm-util';
import { CircleQuestionMark } from 'lucide-react';
import { useCallback, useEffect, useMemo, useState } from 'react';

const STORAGE_KEY = 'ragflow-system-model-settings';

interface ModelFieldItemProps {
  id: string;
  label: string;
  value: string;
  tooltip?: string;
  isRequired?: boolean;
  onChange: (id: string, value: string) => void;
}

/** Read persisted model selections from localStorage */
function loadPersistedValues(): Record<string, string> {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    return raw ? JSON.parse(raw) : {};
  } catch {
    return {};
  }
}

/** Persist model selections to localStorage */
function savePersistedValues(values: Record<string, string>) {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(values));
  } catch {
    // Silently fail if storage is full/disabled
  }
}

function ModelFieldItem({
  label,
  value,
  tooltip,
  id,
  isRequired,
  onChange,
}: ModelFieldItemProps) {
  const { t } = useTranslate('setting');

  return (
    <div className="flex gap-3">
      <label className="block text-sm font-normal text-text-secondary mb-1 w-1/4">
        {isRequired && <span className="text-state-error">*</span>}
        {label}
        {tooltip && (
          <Tooltip>
            <TooltipContent>{tooltip}</TooltipContent>
            <TooltipTrigger>
              <CircleQuestionMark
                size={12}
                className="ml-1 text-text-secondary text-xs"
              />
            </TooltipTrigger>
          </Tooltip>
        )}
      </label>
      <div className="w-3/4">
        <ModelTreeSelect
          modelTypes={ModelTypeMap[id as keyof typeof ModelTypeMap] ?? ['chat']}
          value={value}
          onChange={(val) => onChange(id, val)}
          placeholder={t('selectModelPlaceholder')}
          showSearch
          allowClear={id !== 'llm_id'}
        />
      </div>
    </div>
  );
}

function SystemSetting() {
  const { t } = useTranslate('setting');
  const defaultModelDictionary = useFetchDefaultModelDictionary();
  const { setDefaultModel } = useSetDefaultModel();

  // Local state synced with localStorage for persistence across page refreshes.
  // This handles the case where the Go backend hasn't been rebuilt yet and
  // doesn't return TTS/ASR/VLM defaults from the API (same pattern as ASR/VLM).
  const [persistedValues, setPersistedValues] =
    useState<Record<string, string>>(loadPersistedValues);

  // Sync localStorage whenever persistedValues changes
  useEffect(() => {
    savePersistedValues(persistedValues);
  }, [persistedValues]);

  const handleFieldChange = useCallback(
    async (field: string, value: string) => {
      // Update local state immediately so selection shows right away and persists
      setPersistedValues((prev) => ({ ...prev, [field]: value || '' }));

      const modelType = FieldToModelType[field];
      if (!modelType) return;

      if (!value) {
        await setDefaultModel({
          model_provider: '',
          model_instance: '',
          model_name: '',
          model_type: modelType,
        });
      } else {
        const parsed = parseModelValue(value);
        if (!parsed) return;
        await setDefaultModel({ ...parsed, model_type: modelType });
      }
    },
    [setDefaultModel],
  );

  // Resolution order (same for ALL model types including ASR/VLM/TTS):
  //   1. localStorage (user's latest selection, survives refresh)
  //   2. API response (from Go backend GET /api/v1/models/default)
  //   3. empty string (fallback)
  const getValue = useCallback(
    (field: string) =>
      persistedValues[field] || defaultModelDictionary[field] || '',
    [persistedValues, defaultModelDictionary],
  );

  const llmList = useMemo(() => {
    return [
      {
        id: 'llm_id',
        label: t('chatModel'),
        isRequired: true,
        value: getValue('llm_id'),
        tooltip: t('chatModelTip'),
      },
      {
        id: 'embd_id',
        label: t('embeddingModel'),
        value: getValue('embd_id'),
        tooltip: t('embeddingModelTip'),
      },
      {
        id: 'img2txt_id',
        label: t('img2txtModel'),
        value: getValue('img2txt_id'),
        tooltip: t('img2txtModelTip'),
      },
      {
        id: 'asr_id',
        label: t('sequence2txtModel'),
        value: getValue('asr_id'),
        tooltip: t('sequence2txtModelTip'),
      },
      {
        id: 'rerank_id',
        label: t('rerankModel'),
        value: getValue('rerank_id'),
        tooltip: t('rerankModelTip'),
      },
      {
        id: 'tts_id',
        label: t('ttsModel'),
        value: getValue('tts_id'),
        tooltip: t('ttsModelTip'),
      },
    ];
  }, [getValue, t]);

  return (
    <article className="rounded-lg w-full">
      <header className="py-5">
        <h2 className="text-2xl font-medium text-text-primary">
          {t('systemModelSettings')}
        </h2>
        <p className="mt-1 text-sm text-text-secondary ">
          {t('systemModelDescription')}
        </p>
      </header>

      <div className="px-7 py-6 space-y-6 max-h-[70vh] overflow-y-auto border border-border-button rounded-lg">
        {llmList.map((item) => (
          <ModelFieldItem
            key={item.id}
            {...item}
            onChange={handleFieldChange}
          />
        ))}
      </div>
    </article>
  );
}

export default SystemSetting;
