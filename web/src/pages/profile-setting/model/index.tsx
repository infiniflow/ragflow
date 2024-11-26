import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import {
  AddModelCard,
  ModelLibraryCard,
  SystemModelSetting,
} from './model-card';

const addedModelList = new Array(4).fill(1);

const modelLibraryList = new Array(4).fill(1);

export default function ModelManagement() {
  return (
    <section className="p-8 space-y-8">
      <div className="flex justify-between items-center ">
        <h1 className="text-4xl font-bold">Team management</h1>
        <Button className="hover:bg-[#6B4FD8] text-white bg-colors-background-core-standard">
          Unfinished
        </Button>
      </div>
      <SystemModelSetting></SystemModelSetting>
      <section>
        <h2 className="text-2xl font-semibold mb-3">Added model</h2>
        <div className="grid grid-cols-1 lg:grid-cols-2 xl:grid-cols-4 2xl:grid-cols-4 gap-4">
          {addedModelList.map((x, idx) => (
            <AddModelCard key={idx}></AddModelCard>
          ))}
        </div>
      </section>
      <section>
        <div className="flex justify-between items-center mb-3">
          <h2 className="text-2xl font-semibold ">Model library</h2>
          <Input
            placeholder="search"
            className="bg-colors-background-inverse-weak w-1/5"
          ></Input>
        </div>
        <div className="grid grid-cols-2 lg:grid-cols-4 xl:grid-cols-6 2xl:grid-cols-8 gap-4">
          {modelLibraryList.map((x, idx) => (
            <ModelLibraryCard key={idx}></ModelLibraryCard>
          ))}
        </div>
      </section>
    </section>
  );
}
