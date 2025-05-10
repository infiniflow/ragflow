import { useFetchFlow } from '@/hooks/flow-hooks';
import get from 'lodash/get';
import React, {
  MouseEventHandler,
  useCallback,
  useMemo,
  useState,
} from 'react';
import JsonView from 'react18-json-view';
import 'react18-json-view/src/style.css';
import { useReplaceIdWithText } from '../../hooks';

import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import { useTranslate } from '@/hooks/common-hooks';
import {
  Button,
  Card,
  Col,
  Input,
  Row,
  Space,
  Tabs,
  Typography,
  message,
} from 'antd';
import { useGetComponentLabelByValue } from '../../hooks/use-get-begin-query';

interface IProps extends React.PropsWithChildren {
  nodeId: string;
  name?: string;
}

export function NextNodePopover({ children, nodeId, name }: IProps) {
  const { t } = useTranslate('flow');

  const { data } = useFetchFlow();
  console.log(data);

  const component = useMemo(() => {
    return get(data, ['dsl', 'components', nodeId], {});
  }, [nodeId, data]);

  const inputs: Array<{ component_id: string; content: string }> = get(
    component,
    ['obj', 'inputs'],
    [],
  );
  const output = get(component, ['obj', 'output'], {});
  const { conf, messages, prompt } = get(
    component,
    ['obj', 'params', 'infor'],
    {},
  );
  const { replacedOutput } = useReplaceIdWithText(output);
  const stopPropagation: MouseEventHandler = useCallback((e) => {
    e.stopPropagation();
  }, []);

  const getLabel = useGetComponentLabelByValue(nodeId);

  const [inputPage, setInputPage] = useState(1);
  const pageSize = 3;
  const pagedInputs = inputs.slice(
    (inputPage - 1) * pageSize,
    inputPage * pageSize,
  );

  return (
    <Popover>
      <PopoverTrigger onClick={stopPropagation} asChild>
        {children}
      </PopoverTrigger>
      <PopoverContent
        align={'start'}
        side={'right'}
        sideOffset={20}
        onClick={stopPropagation}
        className="w-[800px] p-4"
        style={{ maxHeight: 600, overflow: 'auto' }}
      >
        <Card
          bordered={false}
          style={{ marginBottom: 16, padding: 0 }}
          bodyStyle={{ padding: 0 }}
        >
          <Typography.Title
            level={5}
            style={{
              marginBottom: 16,
              fontWeight: 600,
              fontSize: 18,
              borderBottom: '1px solid #f0f0f0',
              paddingBottom: 8,
            }}
          >
            {name} {t('operationResults')}
          </Typography.Title>
        </Card>
        <Tabs
          defaultActiveKey="input"
          items={[
            {
              key: 'input',
              label: t('input'),
              children: (
                <Card
                  size="small"
                  className="bg-gray-50 dark:bg-gray-800"
                  style={{ borderRadius: 8, border: '1px solid #e5e7eb' }}
                  bodyStyle={{ padding: 16 }}
                >
                  <Table>
                    <TableHeader>
                      <TableRow>
                        <TableHead>{t('componentId')}</TableHead>
                        <TableHead className="w-[60px]">
                          {t('content')}
                        </TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {pagedInputs.map((x, idx) => (
                        <TableRow key={idx + (inputPage - 1) * pageSize}>
                          <TableCell>{getLabel(x.component_id)}</TableCell>
                          <TableCell className="truncate">
                            {x.content}
                          </TableCell>
                        </TableRow>
                      ))}
                    </TableBody>
                  </Table>
                  {/* Pagination */}
                  {inputs.length > pageSize && (
                    <Row justify="end" style={{ marginTop: 8 }}>
                      <Space>
                        <Button
                          size="small"
                          disabled={inputPage === 1}
                          onClick={() => setInputPage(inputPage - 1)}
                        >
                          Prev
                        </Button>
                        <span className="mx-2 text-sm">
                          {inputPage} / {Math.ceil(inputs.length / pageSize)}
                        </span>
                        <Button
                          size="small"
                          disabled={
                            inputPage === Math.ceil(inputs.length / pageSize)
                          }
                          onClick={() => setInputPage(inputPage + 1)}
                        >
                          Next
                        </Button>
                      </Space>
                    </Row>
                  )}
                </Card>
              ),
            },
            {
              key: 'output',
              label: t('output'),
              children: (
                <Card
                  size="small"
                  className="bg-gray-50 dark:bg-gray-800"
                  style={{ borderRadius: 8, border: '1px solid #e5e7eb' }}
                  bodyStyle={{ padding: 16 }}
                >
                  <JsonView
                    src={replacedOutput}
                    displaySize={30}
                    className="w-full max-h-[300px] break-words overflow-auto"
                  />
                </Card>
              ),
            },
            {
              key: 'infor',
              label: t('infor'),
              children: (
                <Card
                  size="small"
                  className="bg-gray-50 dark:bg-gray-800"
                  style={{ borderRadius: 8, border: '1px solid #e5e7eb' }}
                  bodyStyle={{ padding: 16 }}
                >
                  <Row gutter={16}>
                    <Col span={12}>
                      {conf && (
                        <Card
                          size="small"
                          bordered={false}
                          style={{
                            marginBottom: 16,
                            background: 'transparent',
                          }}
                          bodyStyle={{ padding: 0 }}
                        >
                          <Typography.Text
                            strong
                            style={{
                              color: '#888',
                              marginBottom: 8,
                              display: 'block',
                            }}
                          >
                            Configuration:
                          </Typography.Text>
                          <JsonView
                            src={conf}
                            displaySize={30}
                            className="w-full max-h-[120px] break-words overflow-auto"
                          />
                        </Card>
                      )}
                      {prompt && (
                        <Card
                          size="small"
                          bordered={false}
                          style={{ background: 'transparent' }}
                          bodyStyle={{ padding: 0 }}
                        >
                          <Row
                            align="middle"
                            justify="space-between"
                            style={{ marginBottom: 8 }}
                          >
                            <Col>
                              <Typography.Text strong style={{ color: '#888' }}>
                                Prompt:
                              </Typography.Text>
                            </Col>
                            <Col>
                              <Button
                                size="small"
                                onClick={() => {
                                  const inlineString = prompt
                                    .replace(/\s+/g, ' ')
                                    .trim();
                                  navigator.clipboard.writeText(inlineString);
                                  message.success(
                                    'Prompt copied as single line!',
                                  );
                                }}
                              >
                                Copy as single line
                              </Button>
                            </Col>
                          </Row>
                          <Input.TextArea
                            value={prompt}
                            readOnly
                            autoSize={{ minRows: 2, maxRows: 6 }}
                            className="bg-white dark:bg-gray-900 border-gray-200 dark:border-gray-700"
                          />
                        </Card>
                      )}
                    </Col>
                    <Col span={12}>
                      {messages && (
                        <Card
                          size="small"
                          bordered={false}
                          style={{
                            marginBottom: 16,
                            background: 'transparent',
                          }}
                          bodyStyle={{ padding: 0 }}
                        >
                          <Typography.Text
                            strong
                            style={{
                              color: '#888',
                              marginBottom: 8,
                              display: 'block',
                            }}
                          >
                            Messages:
                          </Typography.Text>
                          <div className="max-h-[300px] overflow-auto">
                            <JsonView
                              src={messages}
                              displaySize={30}
                              className="w-full break-words"
                            />
                          </div>
                        </Card>
                      )}
                    </Col>
                  </Row>
                </Card>
              ),
            },
          ]}
        />
      </PopoverContent>
    </Popover>
  );
}
