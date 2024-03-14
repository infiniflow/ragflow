import { KnowledgeRouteKey } from '@/constants/knowledge';
import { useKnowledgeBaseId } from '@/hooks/knowledgeHook';
import {
  useFetchTenantInfo,
  useSelectParserList,
} from '@/hooks/userSettingHook';
import { Pagination } from '@/interfaces/common';
import { IKnowledgeFile } from '@/interfaces/database/knowledge';
import { getOneNamespaceEffectsLoading } from '@/utils/storeUtil';
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
import { PaginationProps } from 'antd/lib';
import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { useDispatch, useNavigate, useSelector } from 'umi';
import CreateEPModal from './createEFileModal';
import styles from './index.less';
import ParsingActionCell from './parsing-action-cell';
import ParsingStatusCell from './parsing-status-cell';
import RenameModal from './rename-modal';
import SegmentSetModal from './segmentSetModal';

const KnowledgeFile = () => {
  const dispatch = useDispatch();
  const kFModel = useSelector((state: any) => state.kFModel);
  const effects = useSelector((state: any) => state.loading.effects);
  const { data, total } = kFModel;
  const knowledgeBaseId = useKnowledgeBaseId();

  const loading = getOneNamespaceEffectsLoading('kFModel', effects, [
    'getKfList',
    'updateDocumentStatus',
  ]);
  const [doc_id, setDocId] = useState('0');
  const [parser_id, setParserId] = useState('0');
  let navigate = useNavigate();
  const parserList = useSelectParserList();

  const getKfList = useCallback(() => {
    const payload = {
      kb_id: knowledgeBaseId,
    };

    dispatch({
      type: 'kFModel/getKfList',
      payload,
    });
  }, [dispatch, knowledgeBaseId]);

  const throttledGetDocumentList = () => {
    dispatch({
      type: 'kFModel/throttledGetDocumentList',
      payload: knowledgeBaseId,
    });
  };

  const setPagination = useCallback(
    (pageNumber = 1, pageSize?: number) => {
      const pagination: Pagination = {
        current: pageNumber,
      } as Pagination;
      if (pageSize) {
        pagination.pageSize = pageSize;
      }
      dispatch({
        type: 'kFModel/setPagination',
        payload: pagination,
      });
    },
    [dispatch],
  );

  const onPageChange: PaginationProps['onChange'] = useCallback(
    (pageNumber: number, pageSize: number) => {
      setPagination(pageNumber, pageSize);
      getKfList();
    },
    [getKfList, setPagination],
  );

  const pagination: PaginationProps = useMemo(() => {
    return {
      showQuickJumper: true,
      total,
      showSizeChanger: true,
      current: kFModel.pagination.currentPage,
      pageSize: kFModel.pagination.pageSize,
      pageSizeOptions: [1, 2, 10, 20, 50, 100],
      onChange: onPageChange,
    };
  }, [total, kFModel.pagination, onPageChange]);

  useEffect(() => {
    if (knowledgeBaseId) {
      getKfList();
      dispatch({
        type: 'kFModel/pollGetDocumentList-start',
        payload: knowledgeBaseId,
      });
    }
    return () => {
      dispatch({
        type: 'kFModel/pollGetDocumentList-stop',
      });
    };
  }, [knowledgeBaseId, dispatch, getKfList]);

  const handleInputChange = (
    e: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement>,
  ) => {
    const value = e.target.value;
    dispatch({ type: 'kFModel/setSearchString', payload: value });
    setPagination();
    throttledGetDocumentList();
  };

  const onChangeStatus = (e: boolean, doc_id: string) => {
    dispatch({
      type: 'kFModel/updateDocumentStatus',
      payload: {
        doc_id,
        status: Number(e),
        kb_id: knowledgeBaseId,
      },
    });
  };

  const showCEFModal = useCallback(() => {
    dispatch({
      type: 'kFModel/updateState',
      payload: {
        isShowCEFwModal: true,
      },
    });
  }, [dispatch]);

  const linkToUploadPage = useCallback(() => {
    navigate(`/knowledge/dataset/upload?id=${knowledgeBaseId}`);
  }, [navigate, knowledgeBaseId]);

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
        onClick: showCEFModal,
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
  }, [linkToUploadPage, showCEFModal]);

  const toChunk = (id: string) => {
    navigate(
      `/knowledge/${KnowledgeRouteKey.Dataset}/chunk?id=${knowledgeBaseId}&doc_id=${id}`,
    );
  };

  const setDocumentAndParserId = (record: IKnowledgeFile) => () => {
    setDocId(record.id);
    setParserId(record.parser_id);
  };

  const columns: ColumnsType<IKnowledgeFile> = [
    {
      title: 'Name',
      dataIndex: 'name',
      key: 'name',
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
      title: 'Category',
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
          knowledgeBaseId={knowledgeBaseId}
          setDocumentAndParserId={setDocumentAndParserId(record)}
          record={record}
        ></ParsingActionCell>
      ),
    },
  ];

  const finalColumns = columns.map((x) => ({
    ...x,
    className: `${styles.column}`,
  }));

  useFetchTenantInfo();

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
            value={kFModel.searchString}
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
      <CreateEPModal getKfList={getKfList} kb_id={knowledgeBaseId} />
      <SegmentSetModal
        getKfList={getKfList}
        parser_id={parser_id}
        doc_id={doc_id}
      />
      <RenameModal></RenameModal>
    </div>
  );
};

export default KnowledgeFile;
