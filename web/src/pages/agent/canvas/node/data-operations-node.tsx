import { BaseNode } from '@/interfaces/database/agent';
import { NodeProps } from '@xyflow/react';
import { camelCase } from 'lodash';
import { useTranslation } from 'react-i18next';
import { RagNode } from '.';
import { DataOperationsFormSchemaType } from '../../form/data-operations-form';
import { LabelCard } from './card';

export function DataOperationsNode({
  ...props
}: NodeProps<BaseNode<DataOperationsFormSchemaType>>) {
  const { data } = props;
  const { t } = useTranslation();
  const operations = data.form?.operations;

  return (
    <RagNode {...props}>
      <LabelCard>
        {operations && t(`flow.operationsOptions.${camelCase(operations)}`)}
      </LabelCard>
    </RagNode>
  );
}
