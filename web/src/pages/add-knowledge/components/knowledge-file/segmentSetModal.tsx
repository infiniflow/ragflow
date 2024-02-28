import { useFetchParserList, useSelectParserList } from '@/hooks/knowledgeHook';
import { Modal, Space, Tag } from 'antd';
import React, { useEffect, useState } from 'react';
import { useDispatch, useSelector } from 'umi';
import styles from './index.less';
const { CheckableTag } = Tag;
interface kFProps {
  getKfList: () => void;
  parser_id: string;
  doc_id: string;
}
const SegmentSetModal: React.FC<kFProps> = ({
  getKfList,
  parser_id,
  doc_id,
}) => {
  const dispatch = useDispatch();
  const kFModel = useSelector((state: any) => state.kFModel);
  const [selectedTag, setSelectedTag] = useState('');
  const { isShowSegmentSetModal } = kFModel;
  const parserList = useSelectParserList();

  useFetchParserList();

  useEffect(() => {
    setSelectedTag(parser_id);
  }, [parser_id]);

  const handleCancel = () => {
    dispatch({
      type: 'kFModel/updateState',
      payload: {
        isShowSegmentSetModal: false,
      },
    });
  };

  const handleOk = async () => {
    const retcode = await dispatch<any>({
      type: 'kFModel/document_change_parser',
      payload: {
        parser_id: selectedTag,
        doc_id,
      },
    });

    if (retcode === 0 && getKfList) {
      getKfList();
      handleCancel();
    }
  };

  const handleChange = (tag: string, checked: boolean) => {
    const nextSelectedTag = checked ? tag : selectedTag;
    setSelectedTag(nextSelectedTag);
  };

  return (
    <Modal
      title="Parser Type"
      open={isShowSegmentSetModal}
      onOk={handleOk}
      onCancel={handleCancel}
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
export default SegmentSetModal;
