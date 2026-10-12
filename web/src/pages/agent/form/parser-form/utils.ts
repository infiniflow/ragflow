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
// onto the top level and migrates the image family onto its own contract:
// `ocr_enabled` replaces parse_method, and the prompt the image setup used to
// own moves into the global vlm block. Idempotent, so every boundary (form
// defaults, canvas save, dataset load) can apply it: values already carrying a
// top-level enable_vision_enhancement keep it, and per-setup leftovers are always
// stripped (except Audio's vlm, which holds the ASR model). A legacy image
// parse_method that is neither "ocr" nor empty is a VLM model reference: it
// lifts onto vlm.llm_id — deliberately beating an already-set global model, so
// the choice a saved canvas encodes is not lost — and the switch turns off.
// The family language is dropped outright: nothing reads it for images any
// more, and a stored value cannot be told apart from the hardcoded default that
// used to write it (issue #20727).
export function normalizeParserFormValues<T extends Record<string, any>>(
  values: T,
): T & {
  vlm: { llm_id: string; system_prompt: string };
  enable_vision_enhancement: boolean;
} {
  const setups = Array.isArray(values?.setups) ? values.setups : [];
  const visionSetups = setups.filter((x) =>
    VisionEnhancementFileTypes.includes(x?.fileFormat),
  );

  const imageSetup = setups.find((x) => x?.fileFormat === FileType.Image);
  // Mirrors the backend: a setup that already carries the ocr_enabled switch is
  // on the new contract, so any leftover parse_method is dead data — not a model
  // reference. Hoisting it anyway would overwrite the model the user picked.
  const hasImageOcrSwitch = typeof imageSetup?.ocr_enabled === 'boolean';
  const legacyImageMethod = imageSetup?.parse_method;
  const legacyImageModel =
    !hasImageOcrSwitch &&
    typeof legacyImageMethod === 'string' &&
    !isEmpty(legacyImageMethod) &&
    legacyImageMethod.toLowerCase() !== 'ocr'
      ? legacyImageMethod
      : '';

  const enableVisionEnhancement =
    typeof values?.enable_vision_enhancement === 'boolean'
      ? values.enable_vision_enhancement
      : visionSetups.some(
          (x) => x?.flatten_media_to_text === false || !isEmpty(x?.vlm?.llm_id),
        );

  const llmId =
    legacyImageModel ||
    (values?.vlm?.llm_id ??
      visionSetups.find((x) => !isEmpty(x?.vlm?.llm_id))?.vlm?.llm_id ??
      '');

  const nextSetups = setups.map((x) => {
    if (x?.fileFormat === FileType.Audio) return x;
    const stripped = omit(x, ['vlm', 'flatten_media_to_text']);
    if (x?.fileFormat !== FileType.Image) return stripped;
    const { parse_method, ...rest } = omit(stripped, ['lang', 'system_prompt']);
    // Materialize the switch either way. Absent or "ocr" both mean "run local
    // OCR", matching the backend's legacy inference, so the toggle can never
    // display the opposite of what will run. The family language is dropped
    // because nothing reads it any more: captions follow the knowledge base and
    // the only engine that takes a language is MinerU, on the pdf family.
    const runsOCR =
      parse_method === undefined ||
      isEmpty(parse_method) ||
      String(parse_method).toLowerCase() === 'ocr';
    return { ...rest, ocr_enabled: rest.ocr_enabled ?? runsOCR };
  });

  return {
    ...values,
    vlm: {
      // Drop any stored vlm.lang: the enhancement block no longer offers a
      // language, so a stale one in the payload would resurrect a control that
      // nothing renders any more.
      ...omit(values?.vlm, ['lang']),
      llm_id: llmId,
      // Only the prompt lifts: a stored family prompt can only have come from a
      // person, because its default is empty. Language is never lifted — the
      // family value stays where it is as a legacy fallback, and lifting it would
      // turn a machine-written default into an explicit choice. See issue #20727.
      system_prompt:
        values?.vlm?.system_prompt || imageSetup?.system_prompt || '',
    },
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
    vlm: {
      llm_id: defaultModelDictionary[ModelTypeToField.vision] ?? '',
      system_prompt: '',
    },
    setups: initialParserValues.setups.map(
      (setup) =>
        buildInitialParserSetup(
          setup.fileFormat as FileType,
          defaultModelDictionary,
        ) ?? setup,
    ),
  };
}
