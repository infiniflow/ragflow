import { useNextFetchKnowledgeList } from '@/hooks/knowledge-hooks';
import { UserOutlined } from '@ant-design/icons';
import { Avatar, Flex } from 'antd';
import classNames from 'classnames';
import { get } from 'lodash';
import { useMemo } from 'react';
import { Handle, NodeProps, Position } from 'reactflow';
import { NodeData } from '../../interface';
import { LeftHandleStyle, RightHandleStyle } from './handle-icon';
import styles from './index.less';
import NodeHeader from './node-header';
import NodePopover from './popover';

export function RetrievalNode({
  id,
  data,
  isConnectable = true,
  selected,
}: NodeProps<NodeData>) {
  const knowledgeBaseIds: string[] = get(data, 'form.kb_ids', []);
  const { list: knowledgeList } = useNextFetchKnowledgeList(true);
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
    <NodePopover nodeId={id}>
      <section
        className={classNames(styles.logicNode, {
          [styles.selectedNode]: selected,
        })}
      >
        <Handle
          id="c"
          type="source"
          position={Position.Left}
          isConnectable={isConnectable}
          className={styles.handle}
          style={LeftHandleStyle}
        ></Handle>
        <Handle
          type="source"
          position={Position.Right}
          isConnectable={isConnectable}
          className={styles.handle}
          style={RightHandleStyle}
          id="b"
        ></Handle>
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
      </section>
    </NodePopover>
  );
}
