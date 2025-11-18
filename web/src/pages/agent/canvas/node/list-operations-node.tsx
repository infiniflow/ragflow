import { BaseNode } from '@/interfaces/database/agent';
import { NodeProps } from '@xyflow/react';
import { camelCase } from 'lodash';
import { useTranslation } from 'react-i18next';
import { RagNode } from '.';
import { ListOperationsFormSchemaType } from '../../form/list-operations-form';
import { LabelCard } from './card';

export function ListOperationsNode({
  ...props
}: NodeProps<BaseNode<ListOperationsFormSchemaType>>) {
  const { data } = props;
  const { t } = useTranslation();

  return (
    <RagNode {...props}>
      <LabelCard>
        {t(`flow.ListOperationsOptions.${camelCase(data.form?.operations)}`)}
      </LabelCard>
    </RagNode>
  );
}
