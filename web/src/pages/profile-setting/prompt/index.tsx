import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import { Plus, Trash2 } from 'lucide-react';
import { Title } from '../components';

const text = `You are an intelligent assistant. Please summarize the content of the knowledge base to answer the question. Please list the data in the knowledge base and answer in detail. When all knowledge base content is irrelevant to the question, your answer must include the sentence "The answer you are looking for is not found in the knowledge base!" Answers need to consider chat history.
      Here is the knowledge base:
      {knowledge}
      The above is the knowledge base.`;

const PromptManagement = () => {
  const modelLibraryList = new Array(8).fill(1);

  return (
    <div className="p-8 ">
      <div className="mx-auto">
        <div className="flex justify-between items-center mb-8">
          <h1 className="text-4xl font-bold">Prompt templates</h1>
          <Button size={'sm'}>
            <Plus className="mr-2 h-4 w-4" />
            Create template
          </Button>
        </div>
      </div>
      <div className="grid grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 2xl:grid-cols-6 gap-4">
        {modelLibraryList.map((x, idx) => (
          <Card className="p-0" key={idx}>
            <CardContent className="space-y-4 p-4">
              <Title>Prompt name</Title>
              <p className="line-clamp-3">{text}</p>

              <div className="flex justify-end gap-2">
                <Button size={'sm'} variant={'secondary'}>
                  <Trash2 />
                </Button>
                <Button variant={'outline'} size={'sm'}>
                  Edit
                </Button>
              </div>
            </CardContent>
          </Card>
        ))}
      </div>
    </div>
  );
};

export default PromptManagement;
