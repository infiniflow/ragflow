import { render, screen } from '@testing-library/react';
import ToolTimelineItem from './tool-timeline-item';
jest.mock('@/components/originui/timeline', () => {
  const Item = ({ children }: any) => <div>{children}</div>;
  return Object.fromEntries(
    [
      'TimelineContent',
      'TimelineHeader',
      'TimelineIndicator',
      'TimelineItem',
      'TimelineSeparator',
    ].map((name) => [name, Item]),
  );
});
jest.mock('@/components/ui/accordion', () => {
  const Item = ({ children }: any) => <div>{children}</div>;
  return Object.fromEntries(
    ['Accordion', 'AccordionContent', 'AccordionItem', 'AccordionTrigger'].map(
      (name) => [name, Item],
    ),
  );
});
jest.mock('@/components/operator-icon', () => ({
  __esModule: true,
  default: () => null,
  SVGIconMap: {},
}));
jest.mock('../form/components/json-viewer', () => ({ JsonViewer: () => null }));
jest.mock('./workflow-timeline', () => ({
  typeMap: {},
  toLowerCaseStringAndDeleteChar: (value: string) => value,
}));
it('renders shared AnySearch vertical and batch arguments as text', () => {
  render(
    <ToolTimelineItem
      isShare
      sendLoading={false}
      tools={[
        {
          tool_name: 'anysearch_search',
          arguments: {
            params: { library: 'react' },
            queries: [{ query: 'RAGFlow' }],
          },
          status: 'success',
        },
      ]}
    />,
  );
  expect(screen.getByText('{"library":"react"}')).toBeInTheDocument();
  expect(screen.getByText('[{"query":"RAGFlow"}]')).toBeInTheDocument();
});
