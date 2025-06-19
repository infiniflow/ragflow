import { useFetchKnowledgeList } from '@/hooks/knowledge-hooks';
import { IRetrievalNode } from '@/interfaces/database/flow';
import { UserOutlined } from '@ant-design/icons';
import { NodeProps, Position } from '@xyflow/react';
import { Avatar, Flex } from 'antd';
import classNames from 'classnames';
import { get } from 'lodash';
import { memo, useMemo } from 'react';
import { NodeHandleId } from '../../constant';
import { CommonHandle } from './handle';
import { LeftHandleStyle, RightHandleStyle } from './handle-icon';
import styles from './index.less';
import NodeHeader from './node-header';
import { NodeWrapper } from './node-wrapper';
import { ToolBar } from './toolbar';

function InnerRetrievalNode({
  id,
  data,
  isConnectable = true,
  selected,
}: NodeProps<IRetrievalNode>) {
  const knowledgeBaseIds: string[] = get(data, 'form.kb_ids', []);
  const { list: knowledgeList } = useFetchKnowledgeList(true);
  const knowledgeBases = useMemo(() => {
    return knowledgeBaseIds.map((x) => {
      const item = knowledgeList.find((y) => x === y.id);
      return {
        name: item?.name,
        avatar: item?.avatar,
        id: x,
      };
    });
  }, [knowledgeList, knowledgeBaseIds]);

  return (
    <ToolBar selected={selected} id={id} label={data.label}>
      <NodeWrapper>
        <CommonHandle
          id={NodeHandleId.End}
          type="target"
          position={Position.Left}
          isConnectable={isConnectable}
          className={styles.handle}
          style={LeftHandleStyle}
          nodeId={id}
        ></CommonHandle>
        <CommonHandle
          id={NodeHandleId.Start}
          type="source"
          position={Position.Right}
          isConnectable={isConnectable}
          className={styles.handle}
          style={RightHandleStyle}
          nodeId={id}
          isConnectableEnd={false}
        ></CommonHandle>
        <NodeHeader
          id={id}
          name={data.name}
          label={data.label}
          className={classNames({
            [styles.nodeHeader]: knowledgeBaseIds.length > 0,
          })}
        ></NodeHeader>
        <Flex vertical gap={8}>
          {knowledgeBases.map((knowledge) => {
            return (
              <div className={styles.nodeText} key={knowledge.id}>
                <Flex align={'center'} gap={6}>
                  <Avatar
                    size={26}
                    icon={<UserOutlined />}
                    src={knowledge.avatar}
                  />
                  <Flex className={styles.knowledgeNodeName} flex={1}>
                    {knowledge.name}
                  </Flex>
                </Flex>
              </div>
            );
          })}
        </Flex>
      </NodeWrapper>
    </ToolBar>
  );
}

export const RetrievalNode = memo(InnerRetrievalNode);
