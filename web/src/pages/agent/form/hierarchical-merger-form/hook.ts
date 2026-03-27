import { isEmpty } from 'lodash';
import { useEffect, useMemo } from 'react';
import { UseFormReturn, useWatch } from 'react-hook-form';
import { HierarchicalMergerFormSchemaType } from '.';
import {
  Hierarchy,
  initialHierarchicalMergerValues,
} from '../../constant/pipeline';

// type initialValuesType = typeof initialHierarchicalMergerValues;

function transformLevelsToRules(levels: any[]) {
  if (!Array.isArray(levels)) {
    return initialHierarchicalMergerValues.rules;
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

// function isRulesFormatCorrect(rules: any): boolean {
//   if (!rules || !Array.isArray(rules)) {
//     return false;
//   }
//   if (rules.length === 0) {
//     return false;
//   }
//   if (!rules[0] || typeof rules[0] !== 'object') {
//     return false;
//   }
//   if (!Array.isArray(rules[0].levels)) {
//     return false;
//   }
//   return true;
// }

function transformApiResponseToForm(
  apiData: Record<string, any>,
): HierarchicalMergerFormSchemaType {
  if (!apiData) {
    return apiData;
  }

  if (isEmpty(apiData)) {
    return apiData as HierarchicalMergerFormSchemaType;
  }

  const method = apiData.method as 'hierarchy' | 'group';

  let hierarchy = apiData.hierarchy;
  if (typeof hierarchy === 'number') {
    hierarchy = String(hierarchy);
  }

  let rules = apiData.rules;
  const hasLevelsData = apiData.levels && Array.isArray(apiData.levels);

  if (hasLevelsData) {
    rules = transformLevelsToRules(apiData.levels);
  } else if (rules && Array.isArray(rules)) {
    rules = filterEmptyRules(rules);
  }

  //   const rulesFormatCorrect = isRulesFormatCorrect(rules);

  //   if (method === 'group') {
  //     if (rulesFormatCorrect) {
  //       return {
  //         method,
  //         hierarchy,
  //         rules,
  //       };
  //     }
  //     return {
  //       method,
  //       hierarchy,
  //       rules,
  //     };
  //   }

  //   if (rulesFormatCorrect && method === 'hierarchy') {
  //     return {
  //       method,
  //       hierarchy,
  //       rules,
  //     };
  //   }

  return {
    method,
    hierarchy,
    rules,
  };
}

type HierarchyOption = {
  label: string;
  value: Hierarchy;
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
  const rules = useWatch({ name, control: form?.control });
  const currentHierarchy = form.watch('hierarchy');

  const hierarchyOptions = useMemo(() => {
    const maxLevelCount = calculateMaxLevelCount(rules);
    return getDynamicHierarchyOptions(maxLevelCount);
  }, [rules]);

  useEffect(() => {
    if (!currentHierarchy || !form) {
      return;
    }

    const maxOptionValue = hierarchyOptions[hierarchyOptions.length - 1]?.value;

    if (maxOptionValue && currentHierarchy > maxOptionValue) {
      form.setValue('hierarchy', maxOptionValue);
    }
  }, [currentHierarchy, hierarchyOptions, form]);

  return hierarchyOptions;
}

export { transformApiResponseToForm };
