import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import { ChevronRight, Cpu, MessageSquare, Search } from 'lucide-react';

const applications = [
  {
    id: 1,
    title: 'Jarvis chatbot',
    type: 'Chat app',
    date: '11/24/2024',
    icon: <MessageSquare className="h-6 w-6" />,
  },
  {
    id: 2,
    title: 'Search app 01',
    type: 'Search app',
    date: '11/24/2024',
    icon: <Search className="h-6 w-6" />,
  },
  {
    id: 3,
    title: 'Chatbot 01',
    type: 'Chat app',
    date: '11/24/2024',
    icon: <MessageSquare className="h-6 w-6" />,
  },
  {
    id: 4,
    title: 'Workflow 01',
    type: 'Agent',
    date: '11/24/2024',
    icon: <Cpu className="h-6 w-6" />,
  },
];

export function Applications() {
  return (
    <section className="mt-12">
      <div className="flex justify-between items-center mb-6">
        <h2 className="text-2xl font-bold">Applications</h2>
        <div className="flex bg-colors-background-inverse-standard rounded-lg p-1">
          <Button variant="default" size="sm">
            All
          </Button>
          <Button variant="ghost" size="sm">
            Chat
          </Button>
          <Button variant="ghost" size="sm">
            Search
          </Button>
          <Button variant="ghost" size="sm">
            Agents
          </Button>
        </div>
      </div>
      <div className="grid grid-cols-4 gap-6">
        {[...Array(12)].map((_, i) => {
          const app = applications[i % 4];
          return (
            <Card key={i} className="bg-colors-background-inverse-weak">
              <CardContent className="p-4 flex items-center gap-6">
                <div className="w-[70px] h-[70px] rounded-xl flex items-center justify-center bg-gradient-to-br from-[#45A7FA] via-[#AE63E3] to-[#4433FF]">
                  {app.icon}
                </div>
                <div className="flex-1">
                  <h3 className="text-lg font-semibold">{app.title}</h3>
                  <p className="text-sm opacity-80">{app.type}</p>
                  <p className="text-sm opacity-80">{app.date}</p>
                </div>
                <Button variant="secondary" size="icon">
                  <ChevronRight className="h-6 w-6" />
                </Button>
              </CardContent>
            </Card>
          );
        })}
      </div>
    </section>
  );
}
