import { Flex } from 'antd';
import { Operator, operatorMap } from '../../constant';
import OperatorIcon from '../../operator-icon';
import NodeDropdown from './dropdown';

import { useTranslate } from '@/hooks/common-hooks';
import styles from './index.less';
import { NextNodePopover } from './popover';

interface IProps {
  id: string;
  label: string;
  name: string;
  gap?: number;
  className?: string;
}

export function RunStatus({ id, name }: Omit<IProps, 'label'>) {
  const { t } = useTranslate('flow');
  return (
    <section className="flex justify-end items-center pb-1 ">
      <NextNodePopover nodeId={id} name={name}>
        <span className="text-blue-600 cursor-pointer text-[10px]">
          {t('operationResults')}
        </span>
      </NextNodePopover>
    </section>
  );
}

const NodeHeader = ({ label, id, name, gap = 4, className }: IProps) => {
  return (
    <section className="haha">
      {label !== Operator.Answer && <RunStatus id={id} name={name}></RunStatus>}
      <Flex
        flex={1}
        align="center"
        justify={'space-between'}
        gap={gap}
        className={className}
      >
        <OperatorIcon
          name={label as Operator}
          color={operatorMap[label as Operator].color}
        ></OperatorIcon>
        <span className={styles.nodeTitle}>{name}</span>
        <NodeDropdown id={id} label={label}></NodeDropdown>
      </Flex>
    </section>
  );
};

export default NodeHeader;
