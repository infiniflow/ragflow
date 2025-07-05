import { ReactComponent as SelectedFilesCollapseIcon } from '@/assets/svg/selected-files-collapse.svg';
import { useTranslate } from '@/hooks/common-hooks';
import { ITestingChunk, ITestingResult } from '@/interfaces/database/knowledge';
import {
  Card,
  Collapse,
  Empty,
  Flex,
  Image,
  Pagination,
  PaginationProps,
  Space,
} from 'antd';
import camelCase from 'lodash/camelCase';
import SelectFiles from './select-files';

import { useGetPaginationWithRouter } from '@/hooks/logic-hooks';
import { api_host } from '@/utils/api';
import { showImage } from '@/utils/chat';
import { useCallback } from 'react';
import styles from './index.less';

const similarityList: Array<{ field: keyof ITestingChunk; label: string }> = [
  { field: 'similarity', label: 'Hybrid Similarity' },
  { field: 'term_similarity', label: 'Term Similarity' },
  { field: 'vector_similarity', label: 'Vector Similarity' },
];

const ChunkTitle = ({ item }: { item: ITestingChunk }) => {
  const { t } = useTranslate('knowledgeDetails');
  return (
    <Flex gap={10}>
      {similarityList.map((x) => (
        <Space key={x.field}>
          <span className={styles.similarityCircle}>
            {((item[x.field] as number) * 100).toFixed(2)}
          </span>
          <span className={styles.similarityText}>{t(camelCase(x.field))}</span>
        </Space>
      ))}
    </Flex>
  );
};

interface IProps {
  handleTesting: (documentIds?: string[]) => Promise<any>;
  selectedDocumentIds: string[];
  setSelectedDocumentIds: (ids: string[]) => void;
  data?: ITestingResult;
  loading?: boolean;
}

const TestingResult = ({
  handleTesting,
  selectedDocumentIds,
  setSelectedDocumentIds,
  data,
  loading,
}: IProps) => {
  const { documents, chunks, total } = data || {};
  const { t } = useTranslate('knowledgeDetails');
  const { pagination, setPagination } = useGetPaginationWithRouter();

  const onChange: PaginationProps['onChange'] = (pageNumber, pageSize) => {
    pagination.onChange?.(pageNumber, pageSize);
    handleTesting(selectedDocumentIds);
  };

  const onTesting = useCallback(
    (ids: string[]) => {
      setPagination({ page: 1 });
      handleTesting(ids);
    },
    [setPagination, handleTesting],
  );

  return (
    <section className={styles.testingResultWrapper}>
      <Collapse
        expandIcon={() => (
          <SelectedFilesCollapseIcon></SelectedFilesCollapseIcon>
        )}
        className={styles.selectFilesCollapse}
        items={[
          {
            key: '1',
            label: (
              <Flex
                justify={'space-between'}
                align="center"
                className={styles.selectFilesTitle}
              >
                <Space>
                  <span>
                    {selectedDocumentIds?.length ?? 0}/{documents?.length ?? 0}
                  </span>
                  {t('filesSelected')}
                </Space>
              </Flex>
            ),
            children: (
              <div>
                <SelectFiles
                  setSelectedDocumentIds={setSelectedDocumentIds}
                  handleTesting={onTesting}
                ></SelectFiles>
              </div>
            ),
          },
        ]}
      />
      <Flex
        gap={'large'}
        vertical
        flex={1}
        className={styles.selectFilesCollapse}
      >
        {loading === false && chunks && chunks.length > 0 ? (
          chunks?.map((x) => (
            <Card key={x.chunk_id} title={<ChunkTitle item={x}></ChunkTitle>}>
              <div className="flex justify-center">
                {showImage(x.doc_type_kwd) && (
                  <Image
                    id={x.image_id}
                    className={'object-contain max-h-[30vh] w-full text-center'}
                    src={`${api_host}/document/image/${x.image_id}`}
                  ></Image>
                )}
              </div>
              <div className="pt-4">{x.content_with_weight}</div>
            </Card>
          ))
        ) : loading === false && chunks && chunks.length === 0 ? (
          <Empty></Empty>
        ) : null}
      </Flex>
      <Pagination
        {...pagination}
        size={'small'}
        total={total}
        onChange={onChange}
      />
    </section>
  );
};

export default TestingResult;
