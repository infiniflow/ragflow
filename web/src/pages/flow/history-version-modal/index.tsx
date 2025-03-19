import { useTranslate } from '@/hooks/common-hooks';
import { useFetchListVersion, useFetchVersion } from '@/hooks/flow-hooks';
import {
  Background,
  ConnectionMode,
  ReactFlow,
  ReactFlowProvider,
} from '@xyflow/react';
import { Card, Col, Empty, List, Modal, Row, Spin, Typography } from 'antd';
import React, { useState } from 'react';
import { nodeTypes } from '../canvas';

export function useHistoryVersionModal() {
  const [visibleHistoryVersionModal, setVisibleHistoryVersionModal] =
    React.useState(false);

  return {
    visibleHistoryVersionModal,
    setVisibleHistoryVersionModal,
  };
}

type HistoryVersionModalProps = {
  visible: boolean;
  hideModal: () => void;
  id: string;
};

export function HistoryVersionModal({
  visible,
  hideModal,
  id,
}: HistoryVersionModalProps) {
  const { t } = useTranslate('flow');
  const { data, loading } = useFetchListVersion(id);
  const [selectedVersion, setSelectedVersion] = useState<any>(null);
  const { data: flow, loading: loadingVersion } = useFetchVersion(
    selectedVersion?.id,
  );

  React.useEffect(() => {
    if (!loading && data?.length > 0 && !selectedVersion) {
      setSelectedVersion(data[0]);
    }
  }, [data, loading, selectedVersion]);

  const downloadfile = React.useCallback(
    function (e: any) {
      e.stopPropagation();
      console.log('Restore version:', selectedVersion);
      // Create a JSON blob and trigger download
      const jsonContent = JSON.stringify(flow?.dsl.graph, null, 2);
      const blob = new Blob([jsonContent], {
        type: 'application/json',
      });
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = `${selectedVersion.filename || 'flow-version'}-${selectedVersion.id}.json`;
      document.body.appendChild(a);
      a.click();
      document.body.removeChild(a);
      URL.revokeObjectURL(url);
    },
    [selectedVersion, flow?.dsl],
  );
  return (
    <React.Fragment>
      <Modal
        title={t('historyversion')}
        open={visible}
        width={'80vw'}
        onCancel={hideModal}
        footer={null}
        getContainer={() => document.body}
      >
        <Row gutter={16} style={{ height: '60vh' }}>
          <Col span={10} style={{ height: '100%', overflowY: 'auto' }}>
            {loading && <Spin />}
            {!loading && data.length === 0 && (
              <Empty description="No versions found" />
            )}
            {!loading && data.length > 0 && (
              <List
                itemLayout="horizontal"
                dataSource={data}
                pagination={{
                  pageSize: 5,
                  simple: true,
                }}
                renderItem={(item) => (
                  <List.Item
                    key={item.id}
                    onClick={(e) => {
                      e.stopPropagation();
                      setSelectedVersion(item);
                    }}
                    style={{
                      cursor: 'pointer',
                      background:
                        selectedVersion?.id === item.id ? '#f0f5ff' : 'inherit',
                      padding: '8px 12px',
                      borderRadius: '4px',
                    }}
                  >
                    <List.Item.Meta
                      title={`${t('filename')}: ${item.title || '-'}`}
                      description={item.created_at}
                    />
                  </List.Item>
                )}
              />
            )}
          </Col>

          {/* Right panel - Version details */}
          <Col span={14} style={{ height: '100%', overflowY: 'auto' }}>
            {selectedVersion ? (
              <Card title={t('version.details')} bordered={false}>
                <Row gutter={[16, 16]}>
                  {/* Add actions for the selected version (restore, download, etc.) */}
                  <Col span={24}>
                    <div style={{ textAlign: 'right' }}>
                      <Typography.Link onClick={downloadfile}>
                        {t('version.download')}
                      </Typography.Link>
                    </div>
                  </Col>
                </Row>
                <Typography.Title level={4}>
                  {selectedVersion.title || '-'}
                </Typography.Title>

                <Typography.Text
                  type="secondary"
                  style={{ display: 'block', marginBottom: 16 }}
                >
                  {t('version.created')}: {selectedVersion.create_date}
                </Typography.Text>

                {/*render dsl  form api*/}
                {loadingVersion && <Spin />}
                {!loadingVersion && flow?.dsl && (
                  <ReactFlowProvider key={`flow-${selectedVersion.id}`}>
                    <div
                      style={{
                        height: '400px',
                        position: 'relative',
                        zIndex: 0,
                      }}
                    >
                      <ReactFlow
                        connectionMode={ConnectionMode.Loose}
                        nodes={flow?.dsl.graph?.nodes || []}
                        edges={
                          flow?.dsl.graph?.edges.flatMap((x) => ({
                            ...x,
                            type: 'default',
                          })) || []
                        }
                        fitView
                        nodeTypes={nodeTypes}
                        edgeTypes={{}}
                        zoomOnScroll={true}
                        panOnDrag={true}
                        zoomOnDoubleClick={false}
                        preventScrolling={true}
                        minZoom={0.1}
                      >
                        <Background />
                      </ReactFlow>
                    </div>
                  </ReactFlowProvider>
                )}
              </Card>
            ) : (
              <Empty description={t('version.select')} />
            )}
          </Col>
        </Row>
      </Modal>
    </React.Fragment>
  );
}
