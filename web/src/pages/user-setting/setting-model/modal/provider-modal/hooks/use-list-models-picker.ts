import { DynamicFormRef } from '@/components/dynamic-form';
import { useListProviderModels } from '@/hooks/use-llm-request';
import { IModelInfo, IProviderModelItem } from '@/interfaces/request/llm';
import {
  RefObject,
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from 'react';
import type { ProviderConfig } from '../types';

// Derive is_tools from a model descriptor's `features` array. A model is
// considered tool-capable if it advertises either `tool_call` or
// `function_call`. Returns `undefined` when the model has no features info.
const getIsToolsFromFeatures = (
  features: IProviderModelItem['features'],
): boolean | undefined => {
  if (!Array.isArray(features)) return undefined;
  return features.includes('is_tools');
};

// Map a fetched list-model item to the request-side IModelInfo shape.
// Per-model extras (such as is_tools) live under the `extra` object so the
// addProviderInstance API receives them in the shape the backend expects.
const toIModelInfo = (item: IProviderModelItem): IModelInfo => {
  const is_tools = getIsToolsFromFeatures(item.features);
  return {
    model_name: item.name,
    model_type: item.model_types ?? [],
    max_tokens: item.max_tokens ?? 0,
    ...(is_tools !== undefined ? { extra: { is_tools } } : {}),
  };
};

// Compare the model_type sets from an initial IModelInfo entry and a
// freshly fetched list item. Tolerates string-vs-array differences so a
// legacy single-value `model_type` still matches the list's array shape.
const modelTypesMatch = (
  initial: string | string[] | undefined,
  fetched: string[] | undefined,
): boolean => {
  const a = Array.isArray(initial) ? initial : initial ? [initial] : [];
  const b = Array.isArray(fetched) ? fetched : [];
  if (a.length !== b.length) return false;
  const sb = new Set(b);
  return a.every((t) => sb.has(t));
};

interface UseListModelsPickerParams {
  visible: boolean;
  hasModelNameField: boolean;
  editMode?: boolean;
  viewMode?: boolean;
  initialValues?: Record<string, any>;
  llmFactory: string;
  config: ProviderConfig;
  formRef: RefObject<DynamicFormRef>;
}

/**
 * Owns all state for the "List Models" picker:
 * - fetched model catalog (`models`) and loading flag
 * - currently checked items (`selectedModelItems`)
 * - derived `modelInfoList` payload used at verify/submit time
 * - `allSelected` flag and the all/none/individual toggle handlers
 * - the API fetch (with edit-mode pre-check seeding) and modal-reset effect
 *
 * The picker lives entirely in component state — no form fields are touched.
 * A reentrancy ref guards against Radix Checkbox's double-click dispatch
 * (CheckboxBubbleInput re-fires onClick inside a form).
 */
export const useListModelsPicker = ({
  visible,
  hasModelNameField,
  editMode,
  viewMode,
  initialValues,
  llmFactory,
  config,
  formRef,
}: UseListModelsPickerParams) => {
  const [models, setModelsState] = useState<IProviderModelItem[]>([]);
  const [listLoading, setListLoading] = useState(false);
  // Items the user has checked in the picker. Carries the full descriptor
  // (including `features`) so we can derive is_tools per model at submit time.
  const [selectedModelItems, setSelectedModelItemsState] = useState<
    IProviderModelItem[]
  >([]);
  // Edit-mode seed: the model_info array stored on the existing instance.
  // Used to pre-check list items once the model list is fetched.
  const initialModelInfoRef = useRef<IModelInfo[] | null>(null);
  const { listProviderModels } = useListProviderModels();

  // Reentrancy guard for selection toggles. When a Checkbox inside the form is
  // clicked, Radix's CheckboxBubbleInput dispatches a synthetic click on its
  // hidden form input that bubbles up to the row's onClick — so each user
  // click fires the toggle handler twice in the same tick. Calling setState
  // twice (especially for the all/none toggle below) trips React's
  // "Maximum update depth exceeded" guard. The ref short-circuits the second
  // call and is released on the next macrotask.
  const selectionLockRef = useRef(false);

  // Derived: the model_info array passed to verify/submit. One entry per
  // checked list item, with is_tools derived from the model's `features`.
  const modelInfoList: IModelInfo[] = useMemo(
    () => selectedModelItems.map(toIModelInfo),
    [selectedModelItems],
  );

  // "All models" is checked when every fetched model is selected.
  const allSelected = useMemo(
    () => models.length > 0 && selectedModelItems.length === models.length,
    [models.length, selectedModelItems.length],
  );

  // Capture the initial model_info array (edit mode or viewMode) so we
  // can match it against the fetched list once it arrives and pre-check
  // the right items.
  useEffect(() => {
    if (!visible) return;
    initialModelInfoRef.current = null;
    if ((editMode || viewMode) && initialValues) {
      const initial = (initialValues as any).model_info;
      if (Array.isArray(initial)) {
        initialModelInfoRef.current = initial as IModelInfo[];
      } else if (
        (initialValues as any).model_name &&
        (initialValues as any).model_type
      ) {
        // Legacy fallback: a single-model payload may still arrive as flat
        // fields. Wrap it so the matcher below has a uniform input shape.
        initialModelInfoRef.current = [
          {
            model_name: (initialValues as any).model_name,
            model_type: (initialValues as any).model_type,
            max_tokens: (initialValues as any).max_tokens ?? 0,
          },
        ];
      }
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [visible, editMode, viewMode]);

  // Triggered by ToggleList's onOpenChange — fires the API call with the
  // same payload shape as verifyProviderConnection. Caches the result in
  // component state so subsequent opens don't re-fetch. In edit mode, the
  // fetched list is then matched against `initialModelInfoRef` so any model
  // that was already configured is pre-checked.
  const handleListOpenChange = useCallback(
    async (open: boolean) => {
      if (!open || !hasModelNameField) return;
      if (models.length > 0 || listLoading) return;
      setListLoading(true);
      try {
        const rawValues = (formRef.current?.getValues() || {}) as Record<
          string,
          any
        >;
        // In viewMode, pass instance_name so the backend looks up
        // stored credentials server-side (avoids exposing api_key).
        // For new-instance mode, pass api_key/base_url from the form.
        const values =
          viewMode && initialValues
            ? { ...initialValues, ...rawValues }
            : rawValues;
        const verifyArgs = config.verifyTransform
          ? config.verifyTransform({
              ...values,
              model_info: modelInfoList,
            })
          : { apiKey: values.api_key ?? '', baseUrl: values.base_url };
        const res = await listProviderModels({
          provider_name: llmFactory,
          api_key: viewMode ? '' : ((verifyArgs as any).apiKey ?? ''),
          base_url: viewMode ? '' : ((verifyArgs as any).baseUrl ?? ''),
          region: (verifyArgs as any).region,
          model_info: (verifyArgs as any).modelInfo ?? modelInfoList,
          ...(viewMode && initialValues?.instance_name
            ? { instance_name: initialValues.instance_name }
            : {}),
        });
        if (res?.code === 0 && Array.isArray(res.data)) {
          setModelsState(res.data);
          // Edit-mode pre-check: match the initial model_info entries
          // against the freshly fetched list (by name + model_type set)
          // and seed `selectedModelItems`.
          const seed = initialModelInfoRef.current;
          if (seed && seed.length > 0) {
            const matched = res.data.filter((m: IProviderModelItem) =>
              seed.some(
                (s) =>
                  s.model_name === m.name &&
                  modelTypesMatch(s.model_type, m.model_types),
              ),
            );
            if (matched.length > 0) {
              setSelectedModelItemsState(matched);
            }
          }
        }
      } catch (err) {
        console.error('Failed to fetch provider models:', err);
      } finally {
        setListLoading(false);
      }
    },
    [
      hasModelNameField,
      models.length,
      listLoading,
      config,
      listProviderModels,
      llmFactory,
      modelInfoList,
      formRef,
      viewMode,
      initialValues,
    ],
  );

  // Toggling a list item: add it to `selectedModelItems` if absent, remove
  // it otherwise. No form fields are touched — selection lives entirely
  // in component state and is surfaced as `modelInfoList` at verify/submit.
  const handleSelectModel = useCallback((model: IProviderModelItem) => {
    if (selectionLockRef.current) return;
    selectionLockRef.current = true;
    setSelectedModelItemsState((prev) => {
      const idx = prev.findIndex((p) => p.name === model.name);
      if (idx >= 0) {
        const next = prev.slice();
        next.splice(idx, 1);
        return next;
      }
      return [...prev, model];
    });
    setTimeout(() => {
      selectionLockRef.current = false;
    }, 0);
  }, []);

  // Toggling the "All models" row: select every model when none/all are
  // unselected, otherwise clear the selection. Mirrors the per-item toggle
  // semantics so the UI stays consistent (re-clicking all = empty).
  // The reentrancy guard here is critical: without it, the second call sees
  // `prev.length === models.length` (from the first call's `models.slice()`)
  // and returns `[]`, producing two setState calls per click and triggering
  // the "Maximum update depth exceeded" error.
  const handleToggleAll = useCallback(() => {
    if (selectionLockRef.current) return;
    selectionLockRef.current = true;
    setSelectedModelItemsState((prev) => {
      if (prev.length === models.length) {
        return [];
      }
      return models.slice();
    });
    setTimeout(() => {
      selectionLockRef.current = false;
    }, 0);
  }, [models]);

  // Reset everything when the modal is closed.
  useEffect(() => {
    if (!visible) {
      formRef.current?.reset();
      setModelsState([]);
      setSelectedModelItemsState([]);
      setListLoading(false);
      initialModelInfoRef.current = null;
    }
  }, [visible, formRef]);

  const setModels = useCallback(
    (
      updater:
        | IProviderModelItem[]
        | ((prev: IProviderModelItem[]) => IProviderModelItem[]),
    ) => {
      setModelsState(updater);
    },
    [],
  );

  const setSelectedModelItems = useCallback(
    (
      updater:
        | IProviderModelItem[]
        | ((prev: IProviderModelItem[]) => IProviderModelItem[]),
    ) => {
      setSelectedModelItemsState(updater);
    },
    [],
  );

  // --- Model editing ---
  const [editingModel, setEditingModel] = useState<IProviderModelItem | null>(
    null,
  );
  const [editDialogOpen, setEditDialogOpen] = useState(false);

  const handleEditModel = useCallback((model: IProviderModelItem) => {
    setEditingModel(model);
    setEditDialogOpen(true);
  }, []);

  const handleSaveEditedModel = useCallback(
    (updated: IProviderModelItem) => {
      const oldName = editingModel?.name;
      if (!oldName) return;
      const replace = (items: IProviderModelItem[]) =>
        items.map((m) => (m.name === oldName ? updated : m));
      setModelsState(replace);
      setSelectedModelItemsState(replace);
      setEditDialogOpen(false);
      setEditingModel(null);
    },
    [editingModel],
  );

  return {
    models,
    listLoading,
    selectedModelItems,
    modelInfoList,
    allSelected,
    handleListOpenChange,
    handleSelectModel,
    handleToggleAll,
    setModels,
    setSelectedModelItems,
    editingModel,
    editDialogOpen,
    setEditDialogOpen,
    handleEditModel,
    handleSaveEditedModel,
  };
};
