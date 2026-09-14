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
        <dl className="text-sm text-text-primary grid grid-cols-[minmax(20%,auto),1fr] rounded-xl overflow-hidden bg-bg-card">
          {Object.entries<any>(content).map(([key, value]) => (
            <div
              key={key}
              className="contents [:not(:last-child)]>*]border-b-0.5 [:not(:last-child)>*]:border-border-button"
            >
              <dt className="px-4 py-2.5 bg-bg-card">
                <pre>
                  <code>{key}</code>
                </pre>
              </dt>
              <dd className="px-4 py-2.5">
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
        <div className="rounded-xl p-4 bg-bg-card text-sm text-text-primary">
          <pre>
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
