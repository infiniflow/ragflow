import {
  SelectWithSearch,
  SelectWithSearchFlagOptionType,
} from '@/components/originui/select-with-search';
import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Slider } from '@/components/ui/slider';
import { Switch } from '@/components/ui/switch';
import { LlmModelType } from '@/constants/knowledge';
import { useSelectLlmOptionsByModelType } from '@/hooks/use-llm-request';
import { message } from 'antd';
import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import type {
  FieldConfig,
  FieldWeight,
  SearchConfigModalProps,
  SkillSearchConfig,
} from '../types';

// Use SearchConfig as alias for SkillSearchConfig for backward compatibility
type SearchConfig = SkillSearchConfig;

const defaultFieldConfig: FieldConfig = {
  name: { enabled: true, weight: 3.0 },
  tags: { enabled: true, weight: 2.0 },
  description: { enabled: true, weight: 1.0 },
  content: { enabled: false, weight: 0.5 },
};

const defaultConfig: SearchConfig = {
  embd_id: '',
  vector_similarity_weight: 0.3,
  similarity_threshold: 0.2,
  field_config: defaultFieldConfig,
  top_k: 10,
};

export const SearchConfigModal: React.FC<SearchConfigModalProps> = ({
  open,
  onOpenChange,
  config,
  onSave,
  onReindex,
  loading = false,
}) => {
  const { t } = useTranslation();
  const [formData, setFormData] = useState<SearchConfig>(defaultConfig);
  const [saving, setSaving] = useState(false);
  const [reindexing, setReindexing] = useState(false);

  // Get embedding model options from user's configured LLMs
  const llmOptions = useSelectLlmOptionsByModelType();
  const embeddingModelOptions = useMemo(() => {
    return llmOptions[
      LlmModelType.Embedding
    ] as SelectWithSearchFlagOptionType[];
  }, [llmOptions]);

  useEffect(() => {
    if (open) {
      if (config) {
        setFormData({
          ...defaultConfig,
          ...config,
          field_config: {
            ...defaultFieldConfig,
            ...config.field_config,
          },
        });
      } else {
        setFormData(defaultConfig);
      }
    }
  }, [open, config]);

  const handleSave = async () => {
    if (!formData.embd_id) {
      message.error(t('skillSearch.pleaseSelectEmbeddingModel'));
      return;
    }
    setSaving(true);
    try {
      const success = await onSave(formData);
      if (success) {
        onOpenChange(false);
      }
    } finally {
      setSaving(false);
    }
  };

  const handleReindex = async () => {
    if (!onReindex) return;
    if (!formData.embd_id) {
      message.error(t('skillSearch.pleaseSelectEmbeddingModel'));
      return;
    }
    setReindexing(true);
    try {
      await onReindex(formData.embd_id);
    } finally {
      setReindexing(false);
    }
  };

  const updateFieldWeight = (
    field: keyof FieldConfig,
    updates: Partial<FieldWeight>,
  ) => {
    setFormData((prev) => ({
      ...prev,
      field_config: {
        ...prev.field_config,
        [field]: {
          ...prev.field_config[field],
          ...updates,
        },
      },
    }));
  };

  const getSearchTypeLabel = (weight: number) => {
    if (weight === 0) return t('skillSearch.pureKeyword');
    if (weight === 1) return t('skillSearch.pureVector');
    return `${t('skillSearch.hybrid')} (${Math.round((1 - weight) * 100)}% ${t('skillSearch.keyword')} + ${Math.round(weight * 100)}% ${t('skillSearch.vector')})`;
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-2xl max-h-[90vh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle>{t('skillSearch.configTitle')}</DialogTitle>
          <DialogDescription>{t('skillSearch.configDesc')}</DialogDescription>
        </DialogHeader>

        <div className="space-y-6 py-4">
          {/* Embedding Model */}
          <div className="space-y-2">
            <Label htmlFor="embd_id">{t('skillSearch.embeddingModel')}</Label>
            <SelectWithSearch
              value={formData.embd_id}
              onChange={(value) =>
                setFormData((prev) => ({ ...prev, embd_id: value }))
              }
              options={embeddingModelOptions}
              placeholder={t('skillSearch.embeddingModelPlaceholder')}
            />
          </div>

          {/* Hybrid Search Weight */}
          <div className="space-y-4">
            <div className="flex justify-between items-center">
              <Label>{t('skillSearch.vectorSimilarityWeight')}</Label>
              <span className="text-sm text-muted-foreground">
                {getSearchTypeLabel(formData.vector_similarity_weight)}
              </span>
            </div>
            <Slider
              value={[formData.vector_similarity_weight]}
              onValueChange={([value]) =>
                setFormData((prev) => ({
                  ...prev,
                  vector_similarity_weight: value,
                }))
              }
              min={0}
              max={1}
              step={0.1}
            />
            <div className="flex justify-between text-xs text-muted-foreground">
              <span>{t('skillSearch.keywordOnly')}</span>
              <span>{t('skillSearch.balanced')}</span>
              <span>{t('skillSearch.vectorOnly')}</span>
            </div>
          </div>

          {/* Similarity Threshold */}
          <div className="space-y-4">
            <div className="flex justify-between items-center">
              <Label>{t('skillSearch.similarityThreshold')}</Label>
              <span className="text-sm text-muted-foreground">
                {formData.similarity_threshold.toFixed(1)}
              </span>
            </div>
            <Slider
              value={[formData.similarity_threshold]}
              onValueChange={([value]) =>
                setFormData((prev) => ({
                  ...prev,
                  similarity_threshold: value,
                }))
              }
              min={0}
              max={1}
              step={0.05}
            />
          </div>

          {/* Top K */}
          <div className="space-y-2">
            <Label htmlFor="top_k">{t('skillSearch.topK')}</Label>
            <Input
              id="top_k"
              type="number"
              min={1}
              max={100}
              value={formData.top_k}
              onChange={(e) =>
                setFormData((prev) => ({
                  ...prev,
                  top_k: parseInt(e.target.value) || 10,
                }))
              }
            />
          </div>

          {/* Field Configuration */}
          <div className="space-y-4">
            <Label className="text-base font-medium">
              {t('skillSearch.indexFields')}
            </Label>
            <p className="text-sm text-muted-foreground">
              {t('skillSearch.indexFieldsDesc')}
            </p>

            {/* Name Field */}
            <div className="flex items-center justify-between p-3 border rounded-lg">
              <div className="flex items-center gap-3">
                <Switch
                  checked={formData.field_config.name.enabled}
                  onCheckedChange={(checked) =>
                    updateFieldWeight('name', { enabled: checked })
                  }
                />
                <div>
                  <p className="font-medium">{t('skillSearch.fieldName')}</p>
                  <p className="text-xs text-muted-foreground">
                    {t('skillSearch.fieldNameDesc')}
                  </p>
                </div>
              </div>
              <div className="flex items-center gap-2">
                <span className="text-sm text-muted-foreground">
                  {t('skillSearch.weight')}:
                </span>
                <Input
                  type="number"
                  step={0.1}
                  min={0}
                  max={10}
                  value={formData.field_config.name.weight}
                  onChange={(e) =>
                    updateFieldWeight('name', {
                      weight: parseFloat(e.target.value) || 0,
                    })
                  }
                  className="w-20"
                  disabled={!formData.field_config.name.enabled}
                />
              </div>
            </div>

            {/* Tags Field */}
            <div className="flex items-center justify-between p-3 border rounded-lg">
              <div className="flex items-center gap-3">
                <Switch
                  checked={formData.field_config.tags.enabled}
                  onCheckedChange={(checked) =>
                    updateFieldWeight('tags', { enabled: checked })
                  }
                />
                <div>
                  <p className="font-medium">{t('skillSearch.fieldTags')}</p>
                  <p className="text-xs text-muted-foreground">
                    {t('skillSearch.fieldTagsDesc')}
                  </p>
                </div>
              </div>
              <div className="flex items-center gap-2">
                <span className="text-sm text-muted-foreground">
                  {t('skillSearch.weight')}:
                </span>
                <Input
                  type="number"
                  step={0.1}
                  min={0}
                  max={10}
                  value={formData.field_config.tags.weight}
                  onChange={(e) =>
                    updateFieldWeight('tags', {
                      weight: parseFloat(e.target.value) || 0,
                    })
                  }
                  className="w-20"
                  disabled={!formData.field_config.tags.enabled}
                />
              </div>
            </div>

            {/* Description Field */}
            <div className="flex items-center justify-between p-3 border rounded-lg">
              <div className="flex items-center gap-3">
                <Switch
                  checked={formData.field_config.description.enabled}
                  onCheckedChange={(checked) =>
                    updateFieldWeight('description', { enabled: checked })
                  }
                />
                <div>
                  <p className="font-medium">
                    {t('skillSearch.fieldDescription')}
                  </p>
                  <p className="text-xs text-muted-foreground">
                    {t('skillSearch.fieldDescriptionDesc')}
                  </p>
                </div>
              </div>
              <div className="flex items-center gap-2">
                <span className="text-sm text-muted-foreground">
                  {t('skillSearch.weight')}:
                </span>
                <Input
                  type="number"
                  step={0.1}
                  min={0}
                  max={10}
                  value={formData.field_config.description.weight}
                  onChange={(e) =>
                    updateFieldWeight('description', {
                      weight: parseFloat(e.target.value) || 0,
                    })
                  }
                  className="w-20"
                  disabled={!formData.field_config.description.enabled}
                />
              </div>
            </div>

            {/* Content Field */}
            <div className="flex items-center justify-between p-3 border rounded-lg">
              <div className="flex items-center gap-3">
                <Switch
                  checked={formData.field_config.content.enabled}
                  onCheckedChange={(checked) =>
                    updateFieldWeight('content', { enabled: checked })
                  }
                />
                <div>
                  <p className="font-medium">{t('skillSearch.fieldContent')}</p>
                  <p className="text-xs text-muted-foreground">
                    {t('skillSearch.fieldContentDesc')}
                  </p>
                </div>
              </div>
              <div className="flex items-center gap-2">
                <span className="text-sm text-muted-foreground">
                  {t('skillSearch.weight')}:
                </span>
                <Input
                  type="number"
                  step={0.1}
                  min={0}
                  max={10}
                  value={formData.field_config.content.weight}
                  onChange={(e) =>
                    updateFieldWeight('content', {
                      weight: parseFloat(e.target.value) || 0,
                    })
                  }
                  className="w-20"
                  disabled={!formData.field_config.content.enabled}
                />
              </div>
            </div>
          </div>
        </div>

        <DialogFooter className="gap-2">
          {onReindex && (
            <Button
              variant="outline"
              onClick={handleReindex}
              disabled={reindexing || loading}
            >
              {reindexing
                ? t('skillSearch.reindexing')
                : t('skillSearch.reindex')}
            </Button>
          )}
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t('common.cancel')}
          </Button>
          <Button onClick={handleSave} disabled={saving || loading}>
            {saving ? t('common.saving') : t('common.save')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
};

export default SearchConfigModal;
