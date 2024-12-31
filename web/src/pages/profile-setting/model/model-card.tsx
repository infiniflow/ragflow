import { Avatar, AvatarFallback, AvatarImage } from '@/components/ui/avatar';
import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { Key, MoreVertical, Plus, Trash2 } from 'lucide-react';
import { PropsWithChildren } from 'react';

const settings = [
  {
    title: 'GPT Model',
    description:
      'The default chat LLM all the newly created knowledgebase will use.',
    model: 'DeepseekChat',
  },
  {
    title: 'Embedding Model',
    description:
      'The default embedding model all the newly created knowledgebase will use.',
    model: 'DeepseekChat',
  },
  {
    title: 'Image Model',
    description:
      'The default multi-capable model all the newly created knowledgebase will use. It can generate a picture or video.',
    model: 'DeepseekChat',
  },
  {
    title: 'Speech2TXT Model',
    description:
      'The default ASR model all the newly created knowledgebase will use. Use this model to translate voices to text something text.',
    model: 'DeepseekChat',
  },
  {
    title: 'TTS Model',
    description:
      'The default text to speech model all the newly created knowledgebase will use.',
    model: 'DeepseekChat',
  },
];

function Title({ children }: PropsWithChildren) {
  return <span className="font-bold text-xl">{children}</span>;
}

export function SystemModelSetting() {
  return (
    <Card>
      <CardContent className="p-4 space-y-6">
        {settings.map((x, idx) => (
          <div key={idx} className="flex items-center">
            <div className="flex-1 flex flex-col">
              <span className="font-semibold text-base">{x.title}</span>
              <span className="text-colors-text-neutral-standard">
                {x.description}
              </span>
            </div>
            <div className="flex-1">
              <Select defaultValue="english">
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="english">English</SelectItem>
                </SelectContent>
              </Select>
            </div>
          </div>
        ))}
      </CardContent>
    </Card>
  );
}

export function AddModelCard() {
  return (
    <Card className="pt-4">
      <CardContent className="space-y-4">
        <div className="flex justify-between space-y-4">
          <Avatar>
            <AvatarImage src="https://github.com/shadcn.png" alt="@shadcn" />
            <AvatarFallback>CN</AvatarFallback>
          </Avatar>
          <Button variant={'outline'}>Sub models</Button>
        </div>
        <Title>Deep seek</Title>
        <p>LLM,TEXT EMBEDDING, SPEECH2TEXT, MODERATION</p>
        <Card>
          <CardContent className="p-3 flex gap-2">
            <Button variant={'secondary'}>
              deepseek-chat <Trash2 />
            </Button>
            <Button variant={'secondary'}>
              deepseek-code <Trash2 />
            </Button>
          </CardContent>
        </Card>
        <div className="flex justify-end gap-2">
          <Button variant="secondary" size="icon">
            <MoreVertical className="h-4 w-4" />
          </Button>
          <Button variant={'tertiary'}>
            <Key /> API
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}

export function ModelLibraryCard() {
  return (
    <Card className="pt-4">
      <CardContent className="space-y-4">
        <Avatar className="mb-4">
          <AvatarImage src="https://github.com/shadcn.png" alt="@shadcn" />
          <AvatarFallback>CN</AvatarFallback>
        </Avatar>

        <Title>Deep seek</Title>
        <p>LLM,TEXT EMBEDDING, SPEECH2TEXT, MODERATION</p>

        <div className="text-right">
          <Button variant={'tertiary'}>
            <Plus /> Add
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}
