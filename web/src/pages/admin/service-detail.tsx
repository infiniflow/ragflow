import { isPlainObject } from 'lodash';
import { useMemo } from 'react';

import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';

interface ServiceDetailProps {
  content?: any;
}

function ServiceDetail({ content }: ServiceDetailProps) {
  const contentElement = useMemo(() => {
    if (Array.isArray(content) && content.every(isPlainObject)) {
      const headers = Object.keys(content[0]);

      return (
        <Table rootClassName="min-w-max">
          <TableHeader>
            <TableRow>
              {headers.map((header) => (
                <TableHead key={header}>{header}</TableHead>
              ))}
            </TableRow>
          </TableHeader>

          <TableBody>
            {content.map((item) => (
              <TableRow key={item.id as string}>
                {headers.map((header: string) => (
                  <TableCell key={header}>{item[header] as string}</TableCell>
                ))}
              </TableRow>
            ))}
          </TableBody>
        </Table>
      );
    }

    if (isPlainObject(content)) {
      return (
        <dl className="text-sm text-text-primary grid grid-cols-[auto,1fr] border border-card rounded-xl overflow-hidden bg-bg-card">
          {Object.entries<any>(content).map(([key, value]) => (
            <div key={key} className="contents">
              <dt className="px-3 py-2 bg-bg-card">
                <pre>
                  <code>{key}</code>
                </pre>
              </dt>
              <dd className="px-3 py-2">
                <pre>
                  <code>{JSON.stringify(value)}</code>
                </pre>
              </dd>
            </div>
          ))}
        </dl>
      );
    }

    if (typeof content === 'string') {
      return (
        <div className="rounded-lg p-4 border border-border-button bg-bg-input">
          <pre className="text-sm">
            <code>
              {typeof content === 'string'
                ? content
                : JSON.stringify(content, null, 2)}
            </code>
          </pre>
        </div>
      );
    }

    return content;
  }, [content]);

  return contentElement;
}

export default ServiceDetail;
