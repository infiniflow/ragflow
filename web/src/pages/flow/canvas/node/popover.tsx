import { useFetchFlow } from '@/hooks/flow-hooks';
import { Popover } from 'antd';
import get from 'lodash/get';
import React, { useMemo } from 'react';
import JsonView from 'react18-json-view';
import 'react18-json-view/src/style.css';
import { Operator } from '../../constant';
import { useReplaceIdWithText } from '../../hooks';

import styles from './index.less';

interface IProps extends React.PropsWithChildren {
  nodeId: string;
}

const NodePopover = ({ children, nodeId }: IProps) => {
  const { data } = useFetchFlow();
  const component = useMemo(() => {
    return get(data, ['dsl', 'components', nodeId], {});
  }, [nodeId, data]);

  const output = get(component, ['obj', 'params', 'output'], {});
  const componentName = get(component, ['obj', 'component_name'], '');
  const replacedOutput = useReplaceIdWithText(output);

  const content =
    componentName !== Operator.Answer ? (
      <div
        onClick={(e) => {
          e.preventDefault();
          e.stopPropagation();
        }}
      >
        <JsonView
          src={replacedOutput}
          displaySize={30}
          className={styles.jsonView}
        />
      </div>
    ) : undefined;

  return (
    <Popover content={content} placement="right" destroyTooltipOnHide>
      {children}
    </Popover>
  );
};

export default NodePopover;
