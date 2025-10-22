import { IModalProps } from '@/interfaces/common';
import { get_risk_identify } from '@/services/knowledge-service';
import { message, Modal, Upload } from 'antd';
import Dragger from 'antd/es/upload/Dragger';
import { FileSpreadsheet } from 'lucide-react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';

const RiskIdentifyModal = ({
  showRiskModal,
  setShowRiskModal,
  onUploaded,
}: IModalProps<any> & {
  showRiskModal: boolean;
  setShowRiskModal: (value: boolean) => void;
  onUploaded?: (resp?: any) => void;
}) => {
  const { t } = useTranslation();
  const [fileList, setFileList] = useState<any[]>([]);
  const [uploading, setUploading] = useState(false);

  const props = {
    name: 'file',
    multiple: false,
    accept: '.xlsx',
    beforeUpload: (file: File) => {
      const isXlsx =
        file.type ===
        'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet';
      if (!isXlsx) {
        message.error(t('只支持上传xlsx文件'));
        return Upload.LIST_IGNORE;
      }
      setFileList([file]);
      return false; // 阻止自动上传
    },
    onRemove: () => {
      setFileList([]);
    },
    fileList,
    showUploadList: true,
  };

  const handleOk = async () => {
    if (fileList.length === 0) {
      message.warning(t('请先选择xlsx文件'));
      return;
    }
    setUploading(true);
    const formData = new FormData();
    // 从地址栏获取知识库id
    const url = new URL(window.location.href);
    const pathname = url.pathname; // "/dataset/dataset/4e4eb61c822b11f080868b4ffee32926"
    const id = pathname.split('/').pop();
    formData.append('knowledge_base_id', `${id}`);
    formData.append('data', fileList[0]);
    try {
      const res = await get_risk_identify(formData);
      if (res) {
        message.success(t('文件上传成功'));
        setFileList([]);
        setShowRiskModal(false);
        // open retrieval modal after upload success
        onUploaded?.(res);
      } else {
        message.error(t('文件上传失败'));
      }
    } catch (e) {
      message.error(t('文件上传失败'));
    }
    setUploading(false);
  };

  return (
    <Modal
      open={showRiskModal}
      onCancel={() => setShowRiskModal(false)}
      onOk={handleOk}
      okButtonProps={{ loading: uploading }}
    >
      <div className="p-6">
        <h2 className="text-lg font-bold mb-4">
          {t('knowledgeDetails.riskIdentify')}
        </h2>
        <Dragger {...props}>
          <p className="ant-upload-drag-icon">
            <FileSpreadsheet className="size-16" color="#1890fa" />
          </p>
          <p className="ant-upload-text">{t('拖拽或点击上传xlsx文件')}</p>
        </Dragger>
      </div>
    </Modal>
  );
};

export default RiskIdentifyModal;
