import CopyToClipboard from '@/components/copy-to-clipboard';
import { useTranslate } from '@/hooks/common-hooks';
import { IModalProps } from '@/interfaces/common';
import { IToken } from '@/interfaces/database/chat';
import { formatDate } from '@/utils/date';
import { DeleteOutlined } from '@ant-design/icons';
import type { TableProps } from 'antd';
import { Button, Modal, Space, Table } from 'antd';
import { useOperateApiKey } from '../hooks';

const ChatApiKeyModal = ({
  dialogId,
  hideModal,
  idKey,
}: IModalProps<any> & { dialogId?: string; idKey: string }) => {
  const { createToken, removeToken, tokenList, listLoading, creatingLoading } =
    useOperateApiKey(idKey, dialogId);
  const { t } = useTranslate('chat');

  const columns: TableProps<IToken>['columns'] = [
    {
      title: 'Token',
      dataIndex: 'token',
      key: 'token',
      render: (text) => <a>{text}</a>,
    },
    {
      title: t('created'),
      dataIndex: 'create_date',
      key: 'create_date',
      render: (text) => formatDate(text),
    },
    {
      title: t('action'),
      key: 'action',
      render: (_, record) => (
        <Space size="middle">
          <CopyToClipboard text={record.token}></CopyToClipboard>
          <DeleteOutlined onClick={() => removeToken(record.token)} />
        </Space>
      ),
    },
  ];

  return (
    <>
      <Modal
        title={t('apiKey')}
        open
        onCancel={hideModal}
        cancelButtonProps={{ style: { display: 'none' } }}
        style={{ top: 300 }}
        onOk={hideModal}
        width={'50vw'}
      >
        <Table
          columns={columns}
          dataSource={tokenList}
          rowKey={'token'}
          loading={listLoading}
          pagination={false}
        />
        <Button
          onClick={createToken}
          loading={creatingLoading}
          disabled={tokenList?.length > 0}
        >
          {t('createNewKey')}
        </Button>
      </Modal>
    </>
  );
};

export default ChatApiKeyModal;
