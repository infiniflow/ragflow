import { getStructuredDatatype } from '@/utils/canvas-util';
import { get, isPlainObject } from 'lodash';
import { ReactNode, useCallback } from 'react';
import {
  AgentDialogueMode,
  AgentStructuredOutputField,
  BeginId,
  JsonSchemaDataType,
  Operator,
} from '../constant';
import useGraphStore from '../store';

function splitValue(value?: string) {
  return typeof value === 'string' ? value?.split('@') : [];
}
function getNodeId(value: string) {
  return splitValue(value).at(0);
}

export function useShowSecondaryMenu() {
  const { getOperatorTypeFromId, getNode } = useGraphStore((state) => state);

  const showSecondaryMenu = useCallback(
    (value: string, outputLabel: string) => {
      const nodeId = getNodeId(value);
      const operatorType = getOperatorTypeFromId(nodeId);

      // For Agent nodes, show secondary menu for 'structured' field
      if (
        operatorType === Operator.Agent &&
        outputLabel === AgentStructuredOutputField
      ) {
        return true;
      }

      // For Begin nodes in webhook mode, show secondary menu for schema properties (body, headers, query, etc.)
      if (operatorType === Operator.Begin) {
        const node = getNode(nodeId);
        const mode = get(node, 'data.form.mode');
        if (mode === AgentDialogueMode.Webhook) {
          // Check if this output field is from the schema
          const outputs = get(node, 'data.form.outputs', {});
          const outputField = outputs[outputLabel];
          // Show secondary menu if the field is an object or has properties
          return (
            outputField &&
            (outputField.type === 'object' ||
              (outputField.properties &&
                Object.keys(outputField.properties).length > 0))
          );
        }
      }

      return false;
    },
    [getOperatorTypeFromId, getNode],
  );

  return showSecondaryMenu;
}
function useGetBeginOutputsOrSchema() {
  const { getNode } = useGraphStore((state) => state);

  const getBeginOutputs = useCallback(() => {
    const node = getNode(BeginId);
    const outputs = get(node, 'data.form.outputs', {});
    return outputs;
  }, [getNode]);

  const getBeginSchema = useCallback(() => {
    const node = getNode(BeginId);
    const outputs = get(node, 'data.form.schema', {});
    return outputs;
  }, [getNode]);

  return { getBeginOutputs, getBeginSchema };
}

export function useGetStructuredOutputByValue() {
  const { getNode, getOperatorTypeFromId } = useGraphStore((state) => state);

  const { getBeginOutputs } = useGetBeginOutputsOrSchema();

  const getStructuredOutput = useCallback(
    (value: string) => {
      const nodeId = getNodeId(value);
      const node = getNode(nodeId);
      const operatorType = getOperatorTypeFromId(nodeId);
      const fields = splitValue(value);
      const outputLabel = fields.at(1);

      let structuredOutput;
      if (operatorType === Operator.Agent) {
        structuredOutput = get(
          node,
          `data.form.outputs.${AgentStructuredOutputField}`,
        );
      } else if (operatorType === Operator.Begin) {
        // For Begin nodes in webhook mode, get the specific schema property
        const outputs = getBeginOutputs();
        if (outputLabel) {
          structuredOutput = outputs[outputLabel];
        }
      }

      return structuredOutput;
    },
    [getBeginOutputs, getNode, getOperatorTypeFromId],
  );

  return getStructuredOutput;
}

export function useFindAgentStructuredOutputLabel() {
  const getOperatorTypeFromId = useGraphStore(
    (state) => state.getOperatorTypeFromId,
  );

  const findAgentStructuredOutputLabel = useCallback(
    (
      value: string,
      options: Array<{
        label: string;
        value: string;
        parentLabel?: string | ReactNode;
        icon?: ReactNode;
      }>,
    ) => {
      const fields = splitValue(value);
      const operatorType = getOperatorTypeFromId(fields.at(0));

      // Handle Agent structured fields
      if (
        operatorType === Operator.Agent &&
        fields.at(1)?.startsWith(AgentStructuredOutputField)
      ) {
        const agentOption = options.find((x) => value.includes(x.value));
        const jsonSchemaFields = fields
          .at(1)
          ?.slice(AgentStructuredOutputField.length);

        return {
          ...agentOption,
          label: (agentOption?.label ?? '') + jsonSchemaFields,
          value: value,
        };
      }

      // Handle Begin webhook fields
      if (operatorType === Operator.Begin && fields.at(1)) {
        const fieldOption = options
          .filter((x) => x.parentLabel === BeginId)
          .find((x) => value.startsWith(x.value));

        return {
          ...fieldOption,
          label: fields.at(1),
          value: value,
        };
      }
    },
    [getOperatorTypeFromId],
  );

  return findAgentStructuredOutputLabel;
}

export function useFindAgentStructuredOutputTypeByValue() {
  const { getOperatorTypeFromId } = useGraphStore((state) => state);
  const filterStructuredOutput = useGetStructuredOutputByValue();
  const { getBeginSchema } = useGetBeginOutputsOrSchema();

  const findTypeByValue = useCallback(
    (
      values: unknown,
      target: string,
      path: string = '',
    ): string | undefined => {
      const properties =
        get(values, 'properties') || get(values, 'items.properties');

      if (isPlainObject(values) && properties) {
        for (const [key, value] of Object.entries(properties)) {
          const nextPath = path ? `${path}.${key}` : key;
          const { dataType, compositeDataType } = getStructuredDatatype(value);

          if (nextPath === target) {
            return compositeDataType;
          }

          if (
            [JsonSchemaDataType.Object, JsonSchemaDataType.Array].some(
              (x) => x === dataType,
            )
          ) {
            const type = findTypeByValue(value, target, nextPath);
            if (type) {
              return type;
            }
          }
        }
      }
    },
    [],
  );

  const findAgentStructuredOutputTypeByValue = useCallback(
    (value?: string) => {
      if (!value) {
        return;
      }
      const fields = splitValue(value);
      const nodeId = fields.at(0);
      const operatorType = getOperatorTypeFromId(nodeId);
      const jsonSchema = filterStructuredOutput(value);

      // Handle Agent structured fields
      if (
        operatorType === Operator.Agent &&
        fields.at(1)?.startsWith(AgentStructuredOutputField)
      ) {
        const jsonSchemaFields = fields
          .at(1)
          ?.slice(AgentStructuredOutputField.length + 1);

        if (jsonSchemaFields) {
          const type = findTypeByValue(jsonSchema, jsonSchemaFields);
          return type;
        }
      }

      // Handle Begin webhook fields (body, headers, query, etc.)
      if (operatorType === Operator.Begin) {
        const outputLabel = fields.at(1);
        const schema = getBeginSchema();
        if (outputLabel && schema) {
          const jsonSchemaFields = fields.at(1);
          if (jsonSchemaFields) {
            const type = findTypeByValue(schema, jsonSchemaFields);
            return type;
          }
        }
      }
    },
    [
      filterStructuredOutput,
      findTypeByValue,
      getBeginSchema,
      getOperatorTypeFromId,
    ],
  );

  return findAgentStructuredOutputTypeByValue;
}

// TODO: Consider merging with useFindAgentStructuredOutputLabel
export function useFindAgentStructuredOutputLabelByValue() {
  const { getNode } = useGraphStore((state) => state);

  const findAgentStructuredOutputLabel = useCallback(
    (value?: string) => {
      if (value) {
        const operatorName = getNode(getNodeId(value ?? ''))?.data.name;

        if (operatorName) {
          return operatorName + ' / ' + splitValue(value).at(1);
        }
      }

      return '';
    },
    [getNode],
  );

  return findAgentStructuredOutputLabel;
}
