import message from '@/components/ui/message';
import { Spin } from '@/components/ui/spin';
import request from '@/utils/request';
import classNames from 'classnames';
import Papa from 'papaparse';
import React, { useEffect, useRef, useState } from 'react';

interface CSVData {
  rows: string[][];
  headers: string[];
}

interface FileViewerProps {
  className?: string;
  url: string;
}

const CSVFileViewer: React.FC<FileViewerProps> = ({ url }) => {
  const [data, setData] = useState<CSVData | null>(null);
  const [isLoading, setIsLoading] = useState<boolean>(true);
  const containerRef = useRef<HTMLDivElement>(null);
  // const url = useGetDocumentUrl();
  const parseCSV = (csvText: string): CSVData => {
    const result = Papa.parse<string[]>(csvText, {
      header: false,
      skipEmptyLines: false,
    });

    const rows = result.data as string[][];

    const headers = rows[0];
    const dataRows = rows.slice(1);

    return { headers, rows: dataRows };
  };

  useEffect(() => {
    const loadCSV = async () => {
      try {
        const res = await request(url, {
          method: 'GET',
          responseType: 'blob',
          onError: () => {
            message.error('file load failed');
            setIsLoading(false);
          },
        });

        // parse CSV file
        const reader = new FileReader();
        reader.readAsText(res.data);
        reader.onload = () => {
          const parsedData = parseCSV(reader.result as string);
          console.log('file loaded successfully', reader.result);
          setData(parsedData);
        };
      } catch (error) {
        message.error('CSV file parse failed');
        console.error('Error loading CSV file:', error);
      } finally {
        setIsLoading(false);
      }
    };

    loadCSV();

    return () => {
      setData(null);
    };
  }, [url]);

  return (
    <div
      ref={containerRef}
      className={classNames(
        'relative w-full h-full p-4 bg-background-paper border border-border-normal rounded-md',
        'overflow-auto max-h-[80vh] p-2',
      )}
    >
      {isLoading ? (
        <div className="absolute inset-0 flex items-center justify-center">
          <Spin />
        </div>
      ) : data ? (
        <table className="min-w-full divide-y divide-border-normal">
          <thead className="bg-background-header-bar">
            <tr>
              {data.headers.map((header, index) => (
                <th
                  key={`header-${index}`}
                  className="px-6 py-3 text-left text-sm font-medium text-text-primary"
                >
                  {header}
                </th>
              ))}
            </tr>
          </thead>
          <tbody className="bg-background-paper divide-y divide-border-normal">
            {data.rows.map((row, rowIndex) => (
              <tr key={`row-${rowIndex}`}>
                {row.map((cell, cellIndex) => (
                  <td
                    key={`cell-${rowIndex}-${cellIndex}`}
                    className="px-6 py-4 whitespace-nowrap text-sm text-text-secondary"
                  >
                    {cell || '-'}
                  </td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      ) : null}
    </div>
  );
};

export default CSVFileViewer;
