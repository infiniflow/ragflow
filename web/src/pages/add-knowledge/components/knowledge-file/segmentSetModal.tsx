import { Modal, Space, Tag } from 'antd';
import React, { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
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
  const settingModel = useSelector((state: any) => state.settingModel);
  const [selectedTag, setSelectedTag] = useState('');
  const { tenantIfo = {} } = settingModel;
  const { parser_ids = '' } = tenantIfo;
  const { isShowSegmentSetModal } = kFModel;
  const { t } = useTranslation();

  useEffect(() => {
    dispatch({
      type: 'settingModel/getTenantInfo',
      payload: {},
    });
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

    retcode === 0 && getKfList && getKfList();
  };

  const handleChange = (tag: string, checked: boolean) => {
    const nextSelectedTag = checked ? tag : selectedTag;
    console.log('You are interested in: ', nextSelectedTag);
    setSelectedTag(nextSelectedTag);
  };

  return (
    <Modal
      title="Basic Modal"
      open={isShowSegmentSetModal}
      onOk={handleOk}
      onCancel={handleCancel}
    >
      <Space size={[0, 8]} wrap>
        <div className={styles.tags}>
          {parser_ids.split(',').map((tag: string) => {
            return (
              <CheckableTag
                key={tag}
                checked={selectedTag === tag}
                onChange={(checked) => handleChange(tag, checked)}
              >
                {tag}
              </CheckableTag>
            );
          })}
        </div>
      </Space>
    </Modal>
  );
};
export default SegmentSetModal;
