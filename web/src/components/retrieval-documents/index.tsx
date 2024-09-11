import { ReactComponent as SelectedFilesCollapseIcon } from '@/assets/svg/selected-files-collapse.svg';
import { Collapse, Flex, Space } from 'antd';
import SelectFiles from './select-files';

import { useSelectTestingResult } from '@/hooks/knowledge-hooks';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import styles from './index.less';

interface IProps {
  selectedDocumentIdsLength?: number;
  onTesting(documentIds: string[]): void;
}

const RetrievalDocuments = ({ onTesting }: IProps) => {
  const { t } = useTranslation();
  const { documents } = useSelectTestingResult();
  const [selectedDocumentIds, setSelectedDocumentIds] = useState<string[]>([]);

  return (
    <Collapse
      expandIcon={() => <SelectedFilesCollapseIcon></SelectedFilesCollapseIcon>}
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
                  {selectedDocumentIds.length ?? 0}/{documents.length}
                </span>
                {t('knowledgeDetails.filesSelected')}
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
  );
};

export default RetrievalDocuments;
