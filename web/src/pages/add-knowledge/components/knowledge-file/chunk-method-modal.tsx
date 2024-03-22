import { IModalManagerChildrenProps } from '@/components/modal-manager';
import {
  useFetchTenantInfo,
  useSelectParserList,
} from '@/hooks/userSettingHook';
import { Modal, Space, Tag } from 'antd';
import React, { useEffect, useState } from 'react';

import styles from './index.less';

const { CheckableTag } = Tag;

interface IProps extends Omit<IModalManagerChildrenProps, 'showModal'> {
  loading: boolean;
  onOk: (parserId: string) => void;
  showModal?(): void;
  parser_id: string;
}

const ChunkMethodModal: React.FC<IProps> = ({
  parser_id,
  onOk,
  hideModal,
  visible,
}) => {
  const [selectedTag, setSelectedTag] = useState('');
  const parserList = useSelectParserList();

  useFetchTenantInfo();

  useEffect(() => {
    setSelectedTag(parser_id);
  }, [parser_id]);

  const handleOk = async () => {
    onOk(selectedTag);
  };

  const handleChange = (tag: string, checked: boolean) => {
    const nextSelectedTag = checked ? tag : selectedTag;
    setSelectedTag(nextSelectedTag);
  };

  return (
    <Modal
      title="Chunk Method"
      open={visible}
      onOk={handleOk}
      onCancel={hideModal}
    >
      <Space size={[0, 8]} wrap>
        <div className={styles.tags}>
          {parserList.map((x) => {
            return (
              <CheckableTag
                key={x.value}
                checked={selectedTag === x.value}
                onChange={(checked) => handleChange(x.value, checked)}
              >
                {x.label}
              </CheckableTag>
            );
          })}
        </div>
      </Space>
    </Modal>
  );
};
export default ChunkMethodModal;
