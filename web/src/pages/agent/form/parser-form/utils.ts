import { FileType } from '@/constants/file';
import { cloneDeep } from 'lodash';
import {
  FileTypeDefaultModelFieldMap,
  initialParserValues,
} from '../../constant/pipeline';

export function buildFieldNameWithPrefix(name: string, prefix: string) {
  return `${prefix}.${name}`;
}

// Table column settings only take effect on the table parser, so a
// dataset-scoped caller passes its chunk method and gets the fields hidden for
// every other one. A caller that passes nothing (the canvas editor, which owns
// no dataset) keeps the fields visible, preserving the canvas behaviour.
export function isTableColumnSettingsVisible(isTableParser?: boolean) {
  return isTableParser !== false;
}

// Builds the default setup for a file type being added to the parser form,
// prefilling the tenant default model for types that have one (video/audio).
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
// tenant default model prefilled. Only applies to node creation — an existing
// form is shown as saved, so a cleared model stays cleared.
export function buildInitialParserValues(
  defaultModelDictionary: Record<string, string>,
) {
  return {
    ...initialParserValues,
    setups: initialParserValues.setups.map(
      (setup) =>
        buildInitialParserSetup(
          setup.fileFormat as FileType,
          defaultModelDictionary,
        ) ?? setup,
    ),
  };
}
