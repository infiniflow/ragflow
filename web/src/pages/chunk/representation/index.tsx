import { SelectWithSearch } from '@/components/originui/select-with-search';
import { Button } from '@/components/ui/button';
import { TreeDataItem, TreeView } from '@/components/ui/tree-view';
import { Pencil, Search, Trash2 } from 'lucide-react';
import { useCallback, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';

interface RepresentationTemplate {
  id: string;
  name: string;
}

interface RepresentationNode {
  id: string;
  chunkId: string;
  position: string;
  level: string;
  title: string;
  children?: RepresentationNode[];
}

const MockTemplates: RepresentationTemplate[] = [
  { id: 'template-a', name: '知识编译模板A' },
  { id: 'template-b', name: '知识编译模板B' },
];

const MockTreeData: RepresentationNode[] = [
  {
    id: '1',
    chunkId: '#2e9543687325',
    position: 'P1',
    level: 'H1',
    title: '第一章：系统架构总览',
    children: [
      {
        id: '2',
        chunkId: '#2e9543687325',
        position: 'P1',
        level: 'H1',
        title: '第一章：系统架构总览',
        children: [
          {
            id: '3',
            chunkId: '#2e9543687325',
            position: 'P1',
            level: 'H1',
            title: '第一章：系统架构总览',
          },
          {
            id: '4',
            chunkId: '#2e9543687325',
            position: 'P1',
            level: 'H1',
            title: '第一章：系统架构总览',
          },
        ],
      },
    ],
  },
  {
    id: '5',
    chunkId: '#2e9543687325',
    position: 'P1',
    level: 'H1',
    title: '第一章：系统架改构总览',
  },
];

function formatNodeName(node: RepresentationNode): string {
  return `${node.chunkId}    ${node.position}    ${node.level}    ${node.title}`;
}

function convertToTreeData(nodes: RepresentationNode[]): TreeDataItem[] {
  return nodes.map((node) => ({
    id: node.id,
    name: formatNodeName(node),
    actions: (
      <div className="flex items-center gap-1">
        <Button
          variant="ghost"
          size="icon"
          className="h-6 w-6"
          type="button"
          aria-label="edit"
        >
          <Pencil className="h-4 w-4" />
        </Button>
        <Button
          variant="ghost"
          size="icon"
          className="h-6 w-6"
          type="button"
          aria-label="delete"
        >
          <Trash2 className="h-4 w-4" />
        </Button>
      </div>
    ),
    children: node.children ? convertToTreeData(node.children) : undefined,
  }));
}

export default function Representation() {
  const { t } = useTranslation();
  const [selectedTemplate, setSelectedTemplate] = useState<string>(
    MockTemplates[0]!.id,
  );

  const templateOptions = useMemo(
    () =>
      MockTemplates.map((template) => ({
        label: template.name,
        value: template.id,
      })),
    [],
  );

  const treeData = useMemo(() => convertToTreeData(MockTreeData), []);

  const handleSearch = useCallback(() => {
    // TODO: implement search
  }, []);

  const handleSelectChange = useCallback(() => {
    // TODO: implement selection handling
  }, []);

  return (
    <section className="p-5 rounded-2xl">
      <div className="flex items-center justify-between">
        <SelectWithSearch
          options={templateOptions}
          value={selectedTemplate}
          onChange={setSelectedTemplate}
          triggerClassName="w-1/2"
        />
        <Button
          variant="ghost"
          size="icon"
          type="button"
          onClick={handleSearch}
          aria-label={t('chunk.search', 'Search')}
        >
          <Search className="h-5 w-5" />
        </Button>
      </div>
      <div className="mt-6 overflow-auto scrollbar-auto">
        <TreeView
          data={treeData}
          onSelectChange={handleSelectChange}
          expandAll
        />
      </div>
    </section>
  );
}
