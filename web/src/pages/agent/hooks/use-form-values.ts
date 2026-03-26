import { RAGFlowNodeType } from '@/interfaces/database/agent';
import { isEmpty } from 'lodash';
import { useMemo } from 'react';
import { initialTitleChunkerValues } from '../constant/pipeline';

function transformLevelsToRules(levels: any[]) {
  if (!Array.isArray(levels)) {
    return initialTitleChunkerValues.rules;
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

function isRulesFormatCorrect(rules: any): boolean {
  if (!rules || !Array.isArray(rules)) {
    return false;
  }
  if (rules.length === 0) {
    return false;
  }
  if (!rules[0] || typeof rules[0] !== 'object') {
    return false;
  }
  if (!Array.isArray(rules[0].levels)) {
    return false;
  }
  return true;
}

function transformApiResponseToForm(apiData: Record<string, any>) {
  if (!apiData) {
    return null;
  }

  if (isEmpty(apiData)) {
    return null;
  }

  const method = apiData.method as 'group' | 'tree' | 'hierarchy' | undefined;

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

  const rulesFormatCorrect = isRulesFormatCorrect(rules);

  if (method === 'group') {
    if (rulesFormatCorrect) {
      return {
        method,
        hierarchy,
        rules,
      };
    }
    return {
      method,
      hierarchy,
    };
  }

  if (rulesFormatCorrect) {
    return {
      method,
      hierarchy,
      rules,
    };
  }

  return {
    method,
    hierarchy,
  };
}

export function useFormValues(
  defaultValues: Record<string, any>,
  node?: RAGFlowNodeType,
) {
  const values = useMemo(() => {
    const formData = node?.data?.form;

    if (isEmpty(formData)) {
      return defaultValues;
    }

    const transformed = transformApiResponseToForm(formData);

    if (!transformed) {
      return defaultValues;
    }

    return transformed;
  }, [defaultValues, node?.data?.form]);

  return values;
}
