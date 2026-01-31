import { Table } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import React from 'react';

type TranslationTableRow = {
  key: string;
  [language: string]: string;
};

interface TranslationTableProps {
  data: TranslationTableRow[];
  languages: string[];
}

const TranslationTable: React.FC<TranslationTableProps> = ({
  data,
  languages,
}) => {
  // Define columns dynamically based on languages
  const columns: ColumnsType<TranslationTableRow> = [
    {
      title: 'Key',
      dataIndex: 'key',
      key: 'key',
      fixed: 'left',
      width: 200,
      sorter: (a, b) => a.key.localeCompare(b.key), // Sorting by key
    },
    ...languages.map((lang) => ({
      title: lang,
      dataIndex: lang,
      key: lang,
      sorter: (a: any, b: any) => a[lang].localeCompare(b[lang]), // Sorting by language
      // Example filter for each language
      filters: [
        {
          text: 'Show Empty',
          value: 'show_empty',
        },
        {
          text: 'Show Non-Empty',
          value: 'show_non_empty',
        },
      ],
      onFilter: (value: any, record: any) => {
        if (value === 'show_empty') {
          return !record[lang]; // Show rows with empty translations
        }
        if (value === 'show_non_empty') {
          return record[lang] && record[lang].length > 0; // Show rows with non-empty translations
        }
        return true;
      },
    })),
  ];

  return (
    <Table
      columns={columns}
      dataSource={data}
      rowKey="key"
      pagination={{ pageSize: 10 }}
      scroll={{ x: true }}
    />
  );
};

export default TranslationTable;
