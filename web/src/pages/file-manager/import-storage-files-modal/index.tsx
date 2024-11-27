import { IModalManagerChildrenProps } from '@/components/modal-manager';
import SvgIcon from '@/components/svg-icon';
import {
  IImportListResult,
  useFetchImportFileList,
} from '@/hooks/import-file-hooks';
import { useGetRowSelection } from '@/pages/file-manager/hooks';
import { formatNumberWithThousandsSeparator } from '@/utils/common-util';
import { getExtension } from '@/utils/document-util';
import { Flex, Form, Modal, Table, Typography } from 'antd';
import { ColumnsType } from 'antd/es/table';
import { useTranslation } from 'react-i18next';
import styles from './index.less';

const { Text } = Typography;

interface IProps extends Omit<IModalManagerChildrenProps, 'showModal'> {
  loading: boolean;
  onOk: (keys: string[], dir: boolean) => void;
}

const ImportFilesFromStorageModal = ({
  visible,
  hideModal,
  loading,
  onOk,
}: IProps) => {
  const [form] = Form.useForm();

  const { t } = useTranslation('translation', { keyPrefix: 'knowledgeImport' });
  const { rowSelection, setSelectedRowKeys } = useGetRowSelection();
  const handleOk = async () => {
    const ret = await form.validateFields();
    onOk(rowSelection.selectedRowKeys as string[], false);
  };

  const { data, pagination } = useFetchImportFileList();
  const columns: ColumnsType<IImportListResult> = [
    {
      title: t('name'),
      dataIndex: 'name',
      key: 'name',
      fixed: 'left',
      render(value, record) {
        return (
          <Flex gap={0} align="left">
            <SvgIcon name={getExtension(value)} width={50}></SvgIcon>
            <Text ellipsis={{ tooltip: value }}>{value}</Text>
          </Flex>
        );
      },
    },

    {
      title: t('size'),
      dataIndex: 'size',
      width: '20%',
      key: 'size',
      render(value) {
        return (
          formatNumberWithThousandsSeparator((value / 1024).toFixed(2)) + ' KB'
        );
      },
    },
  ];

  return (
    <Modal
      className={styles.importFilesModal}
      width="70%"
      title={t('importFile')}
      open={visible}
      onOk={handleOk}
      onCancel={hideModal}
      okButtonProps={{ loading }}
    >
      <Table
        dataSource={data}
        columns={columns}
        rowKey={'name'}
        rowSelection={rowSelection}
        loading={loading}
        pagination={pagination}
        scroll={{ scrollToFirstRowOnChange: true, x: '100%' }}
      />
    </Modal>
  );
};

export default ImportFilesFromStorageModal;
