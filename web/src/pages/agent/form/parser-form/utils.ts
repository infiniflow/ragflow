import { FileType } from '@/constants/file';
import { ModelTypeToField } from '@/constants/llm';
import cloneDeep from 'lodash/cloneDeep';
import isEmpty from 'lodash/isEmpty';
import omit from 'lodash/omit';
import {
  FileTypeDefaultModelFieldMap,
  initialParserValues,
} from '../../constant/pipeline';

export function buildFieldNameWithPrefix(name: string, prefix: string) {
  return `${prefix}.${name}`;
}

// File types whose image/table blocks go through the vision model when the
// global enhancement is on. Audio is excluded: its per-setup model is an ASR
// model, not a vision one.
export const VisionEnhancementFileTypes: FileType[] = [
  FileType.PDF,
  FileType.Spreadsheet,
  FileType.Doc,
  FileType.Docx,
  FileType.TextMarkdown,
  FileType.Video,
];

// Lifts the legacy per-setup vision options (vlm.llm_id / flatten_media_to_text)
// onto the top level. Idempotent, so every boundary (form defaults, canvas
// save, dataset load) can apply it: values already carrying a top-level
// enable_vision_enhancement keep it, and per-setup leftovers are always
// stripped (except Audio's vlm, which holds the ASR model).
export function normalizeParserFormValues<T extends Record<string, any>>(
  values: T,
): T & { vlm: { llm_id: string }; enable_vision_enhancement: boolean } {
  const setups = Array.isArray(values?.setups) ? values.setups : [];
  const visionSetups = setups.filter((x) =>
    VisionEnhancementFileTypes.includes(x?.fileFormat),
  );

  const enableVisionEnhancement =
    typeof values?.enable_vision_enhancement === 'boolean'
      ? values.enable_vision_enhancement
      : visionSetups.some(
          (x) => x?.flatten_media_to_text === false || !isEmpty(x?.vlm?.llm_id),
        );

  const llmId =
    values?.vlm?.llm_id ??
    visionSetups.find((x) => !isEmpty(x?.vlm?.llm_id))?.vlm?.llm_id ??
    '';

  const nextSetups = setups.map((x) =>
    x?.fileFormat === FileType.Audio
      ? x
      : omit(x, ['vlm', 'flatten_media_to_text']),
  );

  return {
    ...values,
    vlm: { ...values?.vlm, llm_id: llmId },
    enable_vision_enhancement: enableVisionEnhancement,
    setups: nextSetups,
  };
}

// Builds the default setup for a file type being added to the parser form,
// prefilling the tenant default model for types that have one (audio).
export function buildInitialParserSetup(
  fileType: FileType,
  defaultModelDictionary: Record<string, string>,
) {
  const setup = initialParserValues.setups.find(
    (x) => x.fileFormat === fileType,
  );
  if (!setup) {
    return undefined;
  }

  const nextSetup = cloneDeep(setup) as Record<string, any>;
  const field = FileTypeDefaultModelFieldMap[fileType];
  const modelId = field ? defaultModelDictionary[field] : '';
  if (modelId) {
    nextSetup.vlm = { ...nextSetup.vlm, llm_id: modelId };
  }
  return nextSetup;
}

// Form values for a freshly added Parser node: every default file type with its
// tenant default model prefilled, and the global vision model prefilled from
// the tenant's image2text default (enhancement itself stays off). Only applies
// to node creation — an existing form is shown as saved, so a cleared model
// stays cleared.
export function buildInitialParserValues(
  defaultModelDictionary: Record<string, string>,
) {
  return {
    ...initialParserValues,
    vlm: { llm_id: defaultModelDictionary[ModelTypeToField.vision] ?? '' },
    setups: initialParserValues.setups.map(
      (setup) =>
        buildInitialParserSetup(
          setup.fileFormat as FileType,
          defaultModelDictionary,
        ) ?? setup,
    ),
  };
}
