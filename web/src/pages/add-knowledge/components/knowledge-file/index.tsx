import {
  useSelectDocumentList,
  useSetDocumentStatus,
} from '@/hooks/documentHooks';
import { useSelectParserList } from '@/hooks/userSettingHook';
import { IKnowledgeFile } from '@/interfaces/database/knowledge';
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
  Input,
  Space,
  Switch,
  Table,
  Tag,
} from 'antd';
import type { ColumnsType } from 'antd/es/table';
import { useMemo } from 'react';
import ChunkMethodModal from './chunk-method-modal';
import CreateFileModal from './create-file-modal';
import {
  useChangeDocumentParser,
  useCreateEmptyDocument,
  useFetchDocumentListOnMount,
  useGetPagination,
  useHandleSearchChange,
  useNavigateToOtherPage,
  useRenameDocument,
  useSetSelectedRecord,
} from './hooks';
import ParsingActionCell from './parsing-action-cell';
import ParsingStatusCell from './parsing-status-cell';
import RenameModal from './rename-modal';

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
                Local files
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
              Create empty file
            </Button>
          </div>
        ),
        // disabled: true,
      },
    ];
  }, [linkToUploadPage, showCreateModal]);

  const columns: ColumnsType<IKnowledgeFile> = [
    {
      title: 'Name',
      dataIndex: 'name',
      key: 'name',
      fixed: 'left',
      render: (text: any, { id, thumbnail }) => (
        <div className={styles.tochunks} onClick={() => toChunk(id)}>
          <img className={styles.img} src={thumbnail} alt="" />
          {text}
        </div>
      ),
    },
    {
      title: 'Chunk Number',
      dataIndex: 'chunk_num',
      key: 'chunk_num',
    },
    {
      title: 'Upload Date',
      dataIndex: 'create_date',
      key: 'create_date',
    },
    {
      title: 'Chunk Method',
      dataIndex: 'parser_id',
      key: 'parser_id',
      render: (text) => {
        return parserList.find((x) => x.value === text)?.label;
      },
    },
    {
      title: 'Enabled',
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
      title: 'Parsing Status',
      dataIndex: 'run',
      key: 'run',
      render: (text, record) => {
        return <ParsingStatusCell record={record}></ParsingStatusCell>;
      },
    },
    {
      title: 'Action',
      key: 'action',
      render: (_, record) => (
        <ParsingActionCell
          setDocumentAndParserId={setRecord(record)}
          showRenameModal={() => {
            setRecord(record)();
            showRenameModal();
          }}
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
      <h3>Dataset</h3>
      <p>Hey, don't forget to adjust the chunk after adding the dataset! 😉</p>
      <Divider></Divider>
      <div className={styles.filter}>
        <Space>
          <h3>Total</h3>
          <Tag color="purple">{total} files</Tag>
        </Space>
        <Space>
          <Input
            placeholder="Seach your files"
            value={searchString}
            style={{ width: 220 }}
            allowClear
            onChange={handleInputChange}
            prefix={<SearchOutlined />}
          />

          <Dropdown menu={{ items: actionItems }} trigger={['click']}>
            <Button type="primary" icon={<PlusOutlined />}>
              Add file
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
        parserId={currentRecord.parser_id}
        parserConfig={currentRecord.parser_config}
        documentType={currentRecord.type}
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
