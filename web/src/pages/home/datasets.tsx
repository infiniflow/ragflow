import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import { ChevronRight, MoreHorizontal } from 'lucide-react';

const datasets = [
  {
    id: 1,
    title: 'Legal knowledge base',
    files: '1,242 files',
    size: '152 MB',
    created: '12.02.2024',
    image: 'https://github.com/shadcn.png',
  },
  {
    id: 2,
    title: 'HR knowledge base',
    files: '1,242 files',
    size: '152 MB',
    created: '12.02.2024',
    image: 'https://github.com/shadcn.png',
  },
  {
    id: 3,
    title: 'IT knowledge base',
    files: '1,242 files',
    size: '152 MB',
    created: '12.02.2024',
    image: 'https://github.com/shadcn.png',
  },
  {
    id: 4,
    title: 'Legal knowledge base',
    files: '1,242 files',
    size: '152 MB',
    created: '12.02.2024',
    image: 'https://github.com/shadcn.png',
  },
];

export function Datasets() {
  return (
    <section>
      <h2 className="text-2xl font-bold mb-6">Datasets</h2>
      <div className="flex gap-6">
        {datasets.map((dataset) => (
          <Card
            key={dataset.id}
            className="bg-colors-background-inverse-weak flex-1"
          >
            <CardContent className="p-4">
              <div className="flex justify-between mb-4">
                <div
                  className="w-[70px] h-[70px] rounded-xl bg-cover"
                  style={{ backgroundImage: `url(${dataset.image})` }}
                />
                <Button variant="ghost" size="icon">
                  <MoreHorizontal className="h-6 w-6" />
                </Button>
              </div>
              <div className="flex justify-between items-end">
                <div>
                  <h3 className="text-lg font-semibold mb-2">
                    {dataset.title}
                  </h3>
                  <p className="text-sm opacity-80">
                    {dataset.files} | {dataset.size}
                  </p>
                  <p className="text-sm opacity-80">
                    Created {dataset.created}
                  </p>
                </div>
                <Button variant="secondary" size="icon">
                  <ChevronRight className="h-6 w-6" />
                </Button>
              </div>
            </CardContent>
          </Card>
        ))}
        <Button className="h-auto " variant={'tertiary'}>
          See all
        </Button>
      </div>
    </section>
  );
}
