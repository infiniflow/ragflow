import { cloneDeep } from 'lodash';
import { useMemo } from 'react';
import { UseFormReturn, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { TitleChunkerFormSchemaType } from '.';
import {
  Hierarchy,
  originalRules,
  TitleChunkerMethod,
} from '../../constant/pipeline';

function transformLevelsToRules(levels: any[]) {
  if (!Array.isArray(levels)) {
    return originalRules;
  }

  return levels
    .map((levelGroup) => {
      if (Array.isArray(levelGroup)) {
        const filteredExpressions = levelGroup.filter(
          (expression: string) => expression && expression.trim() !== '',
        );
        if (filteredExpressions.length === 0) {
          return null;
        }
        return {
          levels: filteredExpressions.map((expression: string) => ({
            expression,
          })),
        };
      }
      return { levels: [{ expression: '' }] };
    })
    .filter((rule) => rule !== null);
}

function filterEmptyRules(rules: any[]) {
  if (!Array.isArray(rules)) {
    return [];
  }

  return rules
    .map((rule) => {
      if (!rule || !Array.isArray(rule.levels)) {
        return null;
      }
      const filteredLevels = rule.levels.filter(
        (level: any) => level.expression && level.expression.trim() !== '',
      );
      if (filteredLevels.length === 0) {
        return null;
      }
      return { levels: filteredLevels };
    })
    .filter((rule) => rule !== null);
}

function transformApiResponseToForm(
  apiData: Record<string, any>,
): TitleChunkerFormSchemaType {
  const method = apiData.method as TitleChunkerMethod;

  let hierarchy = apiData.hierarchy;
  if (typeof hierarchy === 'number') {
    hierarchy = String(hierarchy);
  }

  // Split hierarchy into two fields by method, and support backward compatibility
  // with the single `hierarchy` field.
  let hierarchyHierarchy = apiData.hierarchyHierarchy;
  let hierarchyGroup = apiData.hierarchyGroup;

  if (!hierarchyHierarchy && !hierarchyGroup && hierarchy) {
    if (method === TitleChunkerMethod.Hierarchy) {
      hierarchyHierarchy = hierarchy;
    } else if (method === TitleChunkerMethod.Group) {
      hierarchyGroup = hierarchy;
    }
  }

  if (!hierarchyGroup) {
    hierarchyGroup = '0';
  }
  if (!hierarchyHierarchy) {
    hierarchyHierarchy = hierarchy || Hierarchy.H3;
  }

  // Extract the new-format rules field, or fall back to legacy formats.
  let rules = apiData.rules;
  // Check whether the API returned the oldest `levels` format (array of string arrays).
  const hasLevelsData = apiData.levels && Array.isArray(apiData.levels);

  if (hasLevelsData) {
    // Convert the legacy `levels` structure into the modern `rules` shape.
    rules = transformLevelsToRules(apiData.levels);
  } else if (rules && Array.isArray(rules)) {
    // Clean up the current-format rules by stripping out empty expressions.
    rules = filterEmptyRules(rules);
  }

  // Backward compatibility: older versions only had a generic `rules` field,
  // while newer versions split it into `hierarchyRules` and `groupRules`.
  // When the backend returns legacy data, migrate the old `rules` to the
  // corresponding new field based on the current `method` so that user
  // configurations are not lost.
  let hierarchyRules = apiData.hierarchyRules;
  let groupRules = apiData.groupRules;

  if (!hierarchyRules) {
    if (method === TitleChunkerMethod.Hierarchy) {
      hierarchyRules = cloneDeep(rules) || cloneDeep(originalRules);
    } else {
      hierarchyRules = cloneDeep(originalRules);
    }
  }

  if (!groupRules) {
    if (method === TitleChunkerMethod.Group) {
      groupRules = cloneDeep(rules) || cloneDeep(originalRules);
    } else {
      groupRules = cloneDeep(originalRules);
    }
  }

  return {
    method,
    hierarchyHierarchy,
    hierarchyGroup,
    include_heading_content: Boolean(apiData.include_heading_content),
    root_chunk_as_heading: Boolean(apiData.root_chunk_as_heading),
    hierarchyRules,
    groupRules,
  };
}

type HierarchyOption = {
  label: string;
  value: string;
};

function getDynamicHierarchyOptions(maxLevel: number): HierarchyOption[] {
  if (maxLevel < 1) {
    maxLevel = 1;
  }
  return Array.from({ length: maxLevel }, (_, i) => ({
    label: `H${i + 1}`,
    value: String(i + 1) as Hierarchy,
  }));
}

function calculateMaxLevelCount(
  rules: Array<{ levels: Array<{ expression: string }> }>,
): number {
  if (!rules || rules.length === 0) {
    return 1;
  }
  return Math.max(...rules.map((rule) => rule.levels.length), 1);
}

export function useDynamicHierarchyOptions(
  form: UseFormReturn<any>,
  name: string,
): HierarchyOption[] {
  const { t } = useTranslation();
  const rules = useWatch({ name, control: form?.control });
  const method = useWatch({ name: 'method', control: form?.control });

  const hierarchyOptions = useMemo(() => {
    const maxLevelCount = calculateMaxLevelCount(rules);
    const options = getDynamicHierarchyOptions(maxLevelCount);

    if (method === TitleChunkerMethod.Group) {
      return [
        { label: t('common.automatic', 'Automatic'), value: '0' },
        ...options,
      ];
    }

    return options;
  }, [method, rules, t]);

  return hierarchyOptions;
}

export { transformApiResponseToForm };
