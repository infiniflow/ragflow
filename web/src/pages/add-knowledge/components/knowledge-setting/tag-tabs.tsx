import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { TagTable } from './tag-table';
import { TagWordCloud } from './tag-word-cloud';

export function TagTabs() {
  return (
    <Tabs defaultValue="account" className="mt-4">
      <TabsList>
        <TabsTrigger value="account">Word cloud</TabsTrigger>
        <TabsTrigger value="password">Table</TabsTrigger>
      </TabsList>
      <TabsContent value="account">
        <TagWordCloud></TagWordCloud>
      </TabsContent>
      <TabsContent value="password">
        <TagTable></TagTable>
      </TabsContent>
    </Tabs>
  );
}
