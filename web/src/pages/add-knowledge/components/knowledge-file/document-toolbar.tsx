import { ReactComponent as CancelIcon } from '@/assets/svg/cancel.svg';
import { ReactComponent as DeleteIcon } from '@/assets/svg/delete.svg';
import { ReactComponent as DisableIcon } from '@/assets/svg/disable.svg';
import { ReactComponent as EnableIcon } from '@/assets/svg/enable.svg';
import { ReactComponent as RunIcon } from '@/assets/svg/run.svg';
import { useShowDeleteConfirm, useTranslate } from '@/hooks/common-hooks';
import {
  useRemoveDocument,
  useRunDocument,
  useSetDocumentStatus,
} from '@/hooks/document-hooks';
import { useGetKnowledgeSearchParams } from '@/hooks/route-hook';
import {
  DownOutlined,
  FileOutlined,
  FileTextOutlined,
  PlusOutlined,
  SearchOutlined,
} from '@ant-design/icons';
import { Button, Dropdown, Flex, Input, MenuProps, Space } from 'antd';
import { useCallback, useMemo } from 'react';
import {
  useFetchDocumentListOnMount,
  useGetPagination,
  useHandleSearchChange,
} from './hooks';
import styles from './index.less';

interface IProps {
  selectedRowKeys: string[];
  showCreateModal(): void;
  showWebCrawlModal(): void;
  showDocumentUploadModal(): void;
}

const DocumentToolbar = ({
  selectedRowKeys,
  showCreateModal,
  showWebCrawlModal,
  showDocumentUploadModal,
}: IProps) => {
  const { t } = useTranslate('knowledgeDetails');
  const { fetchDocumentList } = useFetchDocumentListOnMount();
  const { setPagination, searchString } = useGetPagination(fetchDocumentList);
  const { handleInputChange } = useHandleSearchChange(setPagination);
  const removeDocument = useRemoveDocument();
  const showDeleteConfirm = useShowDeleteConfirm();
  const runDocumentByIds = useRunDocument();
  const { knowledgeId } = useGetKnowledgeSearchParams();
  const changeStatus = useSetDocumentStatus();

  const actionItems: MenuProps['items'] = useMemo(() => {
    return [
      {
        key: '1',
        onClick: showDocumentUploadModal,
        label: (
          <div>
            <Button type="link">
              <Space>
                <FileTextOutlined />
                {t('localFiles')}
              </Space>
            </Button>
          </div>
        ),
      },
      { type: 'divider' },
      // {
      //   key: '2',
      //   onClick: showWebCrawlModal,
      //   label: (
      //     <div>
      //       <Button type="link">
      //         <FileTextOutlined />
      //         {t('webCrawl')}
      //       </Button>
      //     </div>
      //   ),
      // },
      { type: 'divider' },
      {
        key: '3',
        onClick: showCreateModal,
        label: (
          <div>
            <Button type="link">
              <FileOutlined />
              {t('emptyFiles')}
            </Button>
          </div>
        ),
      },
    ];
  }, [showDocumentUploadModal, showCreateModal, t]);

  const handleDelete = useCallback(() => {
    showDeleteConfirm({
      onOk: () => {
        removeDocument(selectedRowKeys);
      },
    });
  }, [removeDocument, showDeleteConfirm, selectedRowKeys]);

  const runDocument = useCallback(
    (run: number) => {
      runDocumentByIds({
        doc_ids: selectedRowKeys,
        run,
        knowledgeBaseId: knowledgeId,
      });
    },
    [runDocumentByIds, selectedRowKeys, knowledgeId],
  );

  const handleRunClick = useCallback(() => {
    runDocument(1);
  }, [runDocument]);

  const handleCancelClick = useCallback(() => {
    runDocument(2);
  }, [runDocument]);

  const onChangeStatus = useCallback(
    (enabled: boolean) => {
      selectedRowKeys.forEach((id) => {
        changeStatus(enabled, id);
      });
    },
    [selectedRowKeys, changeStatus],
  );

  const handleEnableClick = useCallback(() => {
    onChangeStatus(true);
  }, [onChangeStatus]);

  const handleDisableClick = useCallback(() => {
    onChangeStatus(false);
  }, [onChangeStatus]);

  const disabled = selectedRowKeys.length === 0;

  const items: MenuProps['items'] = useMemo(() => {
    return [
      {
        key: '0',
        onClick: handleEnableClick,
        label: (
          <Flex gap={10}>
            <EnableIcon></EnableIcon>
            <b>{t('enabled')}</b>
          </Flex>
        ),
      },
      {
        key: '1',
        onClick: handleDisableClick,
        label: (
          <Flex gap={10}>
            <DisableIcon></DisableIcon>
            <b>{t('disabled')}</b>
          </Flex>
        ),
      },
      { type: 'divider' },
      {
        key: '2',
        onClick: handleRunClick,
        label: (
          <Flex gap={10}>
            <RunIcon></RunIcon>
            <b>{t('run')}</b>
          </Flex>
        ),
      },
      {
        key: '3',
        onClick: handleCancelClick,
        label: (
          <Flex gap={10}>
            <CancelIcon />
            <b>{t('cancel')}</b>
          </Flex>
        ),
      },
      { type: 'divider' },
      {
        key: '4',
        onClick: handleDelete,
        label: (
          <Flex gap={10}>
            <span className={styles.deleteIconWrapper}>
              <DeleteIcon width={18} />
            </span>
            <b>{t('delete', { keyPrefix: 'common' })}</b>
          </Flex>
        ),
      },
    ];
  }, [
    handleDelete,
    handleRunClick,
    handleCancelClick,
    t,
    handleDisableClick,
    handleEnableClick,
  ]);

  return (
    <div className={styles.filter}>
      <Dropdown
        menu={{ items }}
        placement="bottom"
        arrow={false}
        disabled={disabled}
      >
        <Button>
          <Space>
            <b> {t('bulk')}</b>
            <DownOutlined />
          </Space>
        </Button>
      </Dropdown>
      <Space>
        <Input
          placeholder={t('searchFiles')}
          value={searchString}
          style={{ width: 220 }}
          allowClear
          onChange={handleInputChange}
          prefix={<SearchOutlined />}
        />

        <Dropdown menu={{ items: actionItems }} trigger={['click']}>
          <Button type="primary" icon={<PlusOutlined />}>
            {t('addFile')}
          </Button>
        </Dropdown>
      </Space>
    </div>
  );
};

export default DocumentToolbar;
