import ChunkMethodModal from '@/components/chunk-method-modal';
import SvgIcon from '@/components/svg-icon';
import {
  useSelectDocumentList,
  useSetDocumentStatus,
} from '@/hooks/documentHooks';
import { useSelectParserList } from '@/hooks/userSettingHook';
import { IKnowledgeFile } from '@/interfaces/database/knowledge';
import { getExtension } from '@/utils/documentUtils';
import {
  FileOutlined,
  FileTextOutlined,
  PlusOutlined,
  SearchOutlined,
} from '@ant-design/icons';
import type { MenuProps } from 'antd';
import {
  Button,
  Divider,
  Dropdown,
  Flex,
  Input,
  Space,
  Switch,
  Table,
  Tag,
} from 'antd';
import type { ColumnsType } from 'antd/es/table';
import { useMemo } from 'react';
import CreateFileModal from './create-file-modal';
import {
  useChangeDocumentParser,
  useCreateEmptyDocument,
  useFetchDocumentListOnMount,
  useGetPagination,
  useHandleSearchChange,
  useNavigateToOtherPage,
  useRenameDocument,
} from './hooks';
import ParsingActionCell from './parsing-action-cell';
import ParsingStatusCell from './parsing-status-cell';
import RenameModal from './rename-modal';

import { useSetSelectedRecord } from '@/hooks/logicHooks';
import { useTranslation } from 'react-i18next';
import styles from './index.less';

const KnowledgeFile = () => {
  const data = useSelectDocumentList();
  const { fetchDocumentList } = useFetchDocumentListOnMount();
  const parserList = useSelectParserList();
  const { pagination, setPagination, total, searchString } =
    useGetPagination(fetchDocumentList);
  const onChangeStatus = useSetDocumentStatus();
  const { linkToUploadPage, toChunk } = useNavigateToOtherPage();

  const { handleInputChange } = useHandleSearchChange(setPagination);
  const { currentRecord, setRecord } = useSetSelectedRecord();
  const {
    renameLoading,
    onRenameOk,
    renameVisible,
    hideRenameModal,
    showRenameModal,
  } = useRenameDocument(currentRecord.id);
  const {
    createLoading,
    onCreateOk,
    createVisible,
    hideCreateModal,
    showCreateModal,
  } = useCreateEmptyDocument();
  const {
    changeParserLoading,
    onChangeParserOk,
    changeParserVisible,
    hideChangeParserModal,
    showChangeParserModal,
  } = useChangeDocumentParser(currentRecord.id);
  const { t } = useTranslation('translation', {
    keyPrefix: 'knowledgeDetails',
  });

  const actionItems: MenuProps['items'] = useMemo(() => {
    return [
      {
        key: '1',
        onClick: linkToUploadPage,
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
        onClick: showCreateModal,
        label: (
          <div>
            <Button type="link">
              <FileOutlined />
              {t('emptyFiles')}
            </Button>
          </div>
        ),
        // disabled: true,
      },
    ];
  }, [linkToUploadPage, showCreateModal, t]);

  const columns: ColumnsType<IKnowledgeFile> = [
    {
      title: t('name'),
      dataIndex: 'name',
      key: 'name',
      fixed: 'left',
      render: (text: any, { id, thumbnail, name }) => (
        <div className={styles.tochunks} onClick={() => toChunk(id)}>
          <Flex gap={10} align="center">
            {thumbnail ? (
              <img className={styles.img} src={thumbnail} alt="" />
            ) : (
              <SvgIcon
                name={`file-icon/${getExtension(name)}`}
                width={24}
              ></SvgIcon>
            )}
            {text}
          </Flex>
        </div>
      ),
    },
    {
      title: t('chunkNumber'),
      dataIndex: 'chunk_num',
      key: 'chunk_num',
    },
    {
      title: t('uploadDate'),
      dataIndex: 'create_date',
      key: 'create_date',
    },
    {
      title: t('chunkMethod'),
      dataIndex: 'parser_id',
      key: 'parser_id',
      render: (text) => {
        return parserList.find((x) => x.value === text)?.label;
      },
    },
    {
      title: t('enabled'),
      key: 'status',
      dataIndex: 'status',
      render: (_, { status, id }) => (
        <>
          <Switch
            defaultChecked={status === '1'}
            onChange={(e) => {
              onChangeStatus(e, id);
            }}
          />
        </>
      ),
    },
    {
      title: t('parsingStatus'),
      dataIndex: 'run',
      key: 'run',
      render: (text, record) => {
        return <ParsingStatusCell record={record}></ParsingStatusCell>;
      },
    },
    {
      title: t('action'),
      key: 'action',
      render: (_, record) => (
        <ParsingActionCell
          setCurrentRecord={setRecord}
          showRenameModal={showRenameModal}
          showChangeParserModal={showChangeParserModal}
          record={record}
        ></ParsingActionCell>
      ),
    },
  ];

  const finalColumns = columns.map((x) => ({
    ...x,
    className: `${styles.column}`,
  }));

  return (
    <div className={styles.datasetWrapper}>
      <h3>{t('dataset')}</h3>
      <p>{t('datasetDescription')}</p>
      <Divider></Divider>
      <div className={styles.filter}>
        <Space>
          <h3>{t('total', { keyPrefix: 'common' })}</h3>
          <Tag color="purple">{total} files</Tag>
        </Space>
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
      <Table
        rowKey="id"
        columns={finalColumns}
        dataSource={data}
        // loading={loading}
        pagination={pagination}
        scroll={{ scrollToFirstRowOnChange: true, x: 1300, y: 'fill' }}
      />
      <CreateFileModal
        visible={createVisible}
        hideModal={hideCreateModal}
        loading={createLoading}
        onOk={onCreateOk}
      />
      <ChunkMethodModal
        documentId={currentRecord.id}
        parserId={currentRecord.parser_id}
        parserConfig={currentRecord.parser_config}
        documentExtension={getExtension(currentRecord.name)}
        onOk={onChangeParserOk}
        visible={changeParserVisible}
        hideModal={hideChangeParserModal}
        loading={changeParserLoading}
      />
      <RenameModal
        visible={renameVisible}
        onOk={onRenameOk}
        loading={renameLoading}
        hideModal={hideRenameModal}
        initialName={currentRecord.name}
      ></RenameModal>
    </div>
  );
};

export default KnowledgeFile;
