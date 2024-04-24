import { ReactComponent as DeleteIcon } from '@/assets/svg/delete.svg';
import { useTranslate } from '@/hooks/commonHooks';
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
import { useMemo } from 'react';
import {
  useFetchDocumentListOnMount,
  useGetPagination,
  useHandleDeleteFile,
  useHandleSearchChange,
  useSelectBreadcrumbItems,
} from './hooks';

import { Link } from 'umi';
import styles from './index.less';

interface IProps {
  selectedRowKeys: string[];
  showFolderCreateModal: () => void;
}

const itemRender: BreadcrumbProps['itemRender'] = (
  currentRoute,
  params,
  items,
) => {
  const isLast = currentRoute?.path === items[items.length - 1]?.path;

  return isLast ? (
    <span>{currentRoute.title}</span>
  ) : (
    <Link to={`${currentRoute.path}`}>{currentRoute.title}</Link>
  );
};

const FileToolbar = ({ selectedRowKeys, showFolderCreateModal }: IProps) => {
  const { t } = useTranslate('knowledgeDetails');
  const { fetchDocumentList } = useFetchDocumentListOnMount();
  const { setPagination, searchString } = useGetPagination(fetchDocumentList);
  const { handleInputChange } = useHandleSearchChange(setPagination);
  const breadcrumbItems = useSelectBreadcrumbItems();

  const actionItems: MenuProps['items'] = useMemo(() => {
    return [
      {
        key: '1',
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
      {
        key: '2',
        onClick: showFolderCreateModal,
        label: (
          <div>
            <Button type="link">
              <FolderOpenOutlined />
              New Folder
            </Button>
          </div>
        ),
        // disabled: true,
      },
    ];
  }, [t, showFolderCreateModal]);

  const { handleRemoveFile } = useHandleDeleteFile(selectedRowKeys);

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
    ];
  }, [handleRemoveFile, t]);

  return (
    <div className={styles.filter}>
      <Breadcrumb items={breadcrumbItems} itemRender={itemRender} />
      <Space>
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

export default FileToolbar;
