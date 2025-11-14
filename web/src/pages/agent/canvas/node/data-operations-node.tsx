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

  return (
    <RagNode {...props}>
      <LabelCard>
        {t(`flow.operationsOptions.${camelCase(data.form?.operations)}`)}
      </LabelCard>
    </RagNode>
  );
}
