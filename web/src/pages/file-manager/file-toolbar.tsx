import { ReactComponent as DeleteIcon } from '@/assets/svg/delete.svg';
import SvgIcon from '@/components/svg-icon';
import { useTranslate } from '@/hooks/common-hooks';
import {
  IListResult,
  useFetchParentFolderList,
} from '@/hooks/file-manager-hooks';
import {
  DownOutlined,
  FileTextOutlined,
  FolderOpenOutlined,
  PlusOutlined,
  SearchOutlined,
} from '@ant-design/icons';
import {
  Breadcrumb,
  BreadcrumbProps,
  Button,
  Dropdown,
  Flex,
  Input,
  MenuProps,
  Space,
} from 'antd';
import { useCallback, useMemo } from 'react';
import {
  useHandleBreadcrumbClick,
  useHandleDeleteFile,
  useSelectBreadcrumbItems,
} from './hooks';

import styles from './index.less';

interface IProps
  extends Pick<IListResult, 'searchString' | 'handleInputChange'> {
  selectedRowKeys: string[];
  showFolderCreateModal: () => void;
  showFileUploadModal: () => void;
  setSelectedRowKeys: (keys: string[]) => void;
  showMoveFileModal: (ids: string[]) => void;
}

const FileToolbar = ({
  selectedRowKeys,
  showFolderCreateModal,
  showFileUploadModal,
  setSelectedRowKeys,
  searchString,
  handleInputChange,
  showMoveFileModal,
}: IProps) => {
  const { t } = useTranslate('knowledgeDetails');
  const breadcrumbItems = useSelectBreadcrumbItems();
  const { handleBreadcrumbClick } = useHandleBreadcrumbClick();
  const parentFolderList = useFetchParentFolderList();
  const isKnowledgeBase =
    parentFolderList.at(-1)?.source_type === 'knowledgebase';

  const itemRender: BreadcrumbProps['itemRender'] = (
    currentRoute,
    params,
    items,
  ) => {
    const isLast = currentRoute?.path === items[items.length - 1]?.path;

    return isLast ? (
      <span>{currentRoute.title}</span>
    ) : (
      <span
        className={styles.breadcrumbItemButton}
        onClick={() => handleBreadcrumbClick(currentRoute.path)}
      >
        {currentRoute.title}
      </span>
    );
  };

  const actionItems: MenuProps['items'] = useMemo(() => {
    return [
      {
        key: '1',
        onClick: showFileUploadModal,
        label: (
          <div>
            <Button type="link">
              <Space>
                <FileTextOutlined />
                {t('uploadFile', { keyPrefix: 'fileManager' })}
              </Space>
            </Button>
          </div>
        ),
      },
      { type: 'divider' },
      {
        key: '2',
        onClick: showFolderCreateModal,
        label: (
          <div>
            <Button type="link">
              <Space>
                <FolderOpenOutlined />
                {t('newFolder', { keyPrefix: 'fileManager' })}
              </Space>
            </Button>
          </div>
        ),
      },
    ];
  }, [t, showFolderCreateModal, showFileUploadModal]);

  const { handleRemoveFile } = useHandleDeleteFile(
    selectedRowKeys,
    setSelectedRowKeys,
  );

  const handleShowMoveFileModal = useCallback(() => {
    showMoveFileModal(selectedRowKeys);
  }, [selectedRowKeys, showMoveFileModal]);

  const disabled = selectedRowKeys.length === 0;

  const items: MenuProps['items'] = useMemo(() => {
    return [
      {
        key: '4',
        onClick: handleRemoveFile,
        label: (
          <Flex gap={10}>
            <span className={styles.deleteIconWrapper}>
              <DeleteIcon width={18} />
            </span>
            <b>{t('delete', { keyPrefix: 'common' })}</b>
          </Flex>
        ),
      },
      {
        key: '5',
        onClick: handleShowMoveFileModal,
        label: (
          <Flex gap={10}>
            <span className={styles.deleteIconWrapper}>
              <SvgIcon name={`move`} width={18}></SvgIcon>
            </span>
            <b>{t('move', { keyPrefix: 'common' })}</b>
          </Flex>
        ),
      },
    ];
  }, [handleShowMoveFileModal, t, handleRemoveFile]);

  return (
    <div className={styles.filter}>
      <Breadcrumb items={breadcrumbItems} itemRender={itemRender} />
      <Space>
        {isKnowledgeBase || (
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
        )}
        <Input
          placeholder={t('searchFiles')}
          value={searchString}
          style={{ width: 220 }}
          allowClear
          onChange={handleInputChange}
          prefix={<SearchOutlined />}
        />

        {isKnowledgeBase || (
          <Dropdown menu={{ items: actionItems }} trigger={['click']}>
            <Button type="primary" icon={<PlusOutlined />}>
              {t('addFile')}
            </Button>
          </Dropdown>
        )}
      </Space>
    </div>
  );
};

export default FileToolbar;
