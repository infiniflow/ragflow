// src/pages/next-search/search-setting.tsx

import { RAGFlowAvatar } from '@/components/ragflow-avatar';
import { Button } from '@/components/ui/button';
import { SingleFormSlider } from '@/components/ui/dual-range-slider';
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import {
  MultiSelect,
  MultiSelectOptionType,
} from '@/components/ui/multi-select';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { Switch } from '@/components/ui/switch';
import { useFetchKnowledgeList } from '@/hooks/knowledge-hooks';
import { IKnowledge } from '@/interfaces/database/knowledge';
import { cn } from '@/lib/utils';
import { transformFile2Base64 } from '@/utils/file-util';
import { t } from 'i18next';
import { PanelRightClose, Pencil, Upload } from 'lucide-react';
import { useEffect, useState } from 'react';
import { useForm } from 'react-hook-form';
import { ISearchAppDetailProps } from '../next-searches/hooks';

interface SearchSettingProps {
  open: boolean;
  setOpen: (open: boolean) => void;
  className?: string;
  data: ISearchAppDetailProps;
}

const SearchSetting: React.FC<SearchSettingProps> = ({
  open = false,
  setOpen,
  className,
  data,
}) => {
  const [width0, setWidth0] = useState('w-[440px]');
  //  "avatar": null,
  // "created_by": "c3fb861af27a11efa69751e139332ced",
  // "description": "My first search app",
  // "id": "22e874584b4511f0aa1ac57b9ea5a68b",
  // "name": "updated search app",
  // "search_config": {
  //     "cross_languages": [],
  //     "doc_ids": [],
  //     "highlight": false,
  //     "kb_ids": [],
  //     "keyword": false,
  //     "query_mindmap": false,
  //     "related_search": false,
  //     "rerank_id": "",
  //     "similarity_threshold": 0.5,
  //     "summary": false,
  //     "top_k": 1024,
  //     "use_kg": true,
  //     "vector_similarity_weight": 0.3,
  //     "web_search": false
  // },
  // "tenant_id": "c3fb861af27a11efa69751e139332ced",
  // "update_time": 1750144129641
  const formMethods = useForm({
    defaultValues: {
      id: '',
      name: '',
      avatar: '',
      description: 'You are an intelligent assistant.',
      datasets: '',
      keywordSimilarityWeight: 20,
      rerankModel: false,
      aiSummary: false,
      topK: true,
      searchMethod: '',
      model: '',
      enableWebSearch: false,
      enableRelatedSearch: true,
      showQueryMindmap: true,
    },
  });
  const [avatarFile, setAvatarFile] = useState<File | null>(null);
  const [avatarBase64Str, setAvatarBase64Str] = useState(''); // Avatar Image base64
  const [datasetList, setDatasetList] = useState<MultiSelectOptionType[]>([]);
  const [datasetSelectEmbdId, setDatasetSelectEmbdId] = useState('');
  useEffect(() => {
    if (!open) {
      setTimeout(() => {
        setWidth0('w-0 hidden');
      }, 500);
    } else {
      setWidth0('w-[440px]');
    }
  }, [open]);
  useEffect(() => {
    if (!avatarFile) {
      setAvatarBase64Str(data?.avatar);
    }
  }, [avatarFile, data?.avatar]);
  useEffect(() => {
    if (avatarFile) {
      (async () => {
        // make use of img compression transformFile2Base64
        setAvatarBase64Str(await transformFile2Base64(avatarFile));
      })();
    }
  }, [avatarFile]);
  const { list: datasetListOrigin } = useFetchKnowledgeList();

  useEffect(() => {
    const datasetListMap = datasetListOrigin.map((item: IKnowledge) => {
      return {
        label: item.name,
        suffix: (
          <div className="text-xs px-4 p-1 bg-bg-card text-text-secondary rounded-lg border border-bg-card">
            {item.embd_id}
          </div>
        ),
        value: item.id,
        disabled:
          item.embd_id !== datasetSelectEmbdId && datasetSelectEmbdId !== '',
      };
    });
    setDatasetList(datasetListMap);
  }, [datasetListOrigin, datasetSelectEmbdId]);

  const handleDatasetSelectChange = (value, onChange) => {
    console.log(value);
    if (value.length) {
      const data = datasetListOrigin?.find((item) => item.id === value[0]);
      setDatasetSelectEmbdId(data?.embd_id ?? '');
    } else {
      setDatasetSelectEmbdId('');
    }
    onChange?.(value);
  };
  return (
    <div
      className={cn(
        'text-text-primary border p-4 rounded-lg',
        {
          'animate-fade-in-right': open,
          'animate-fade-out-right': !open,
        },
        width0,
        className,
      )}
      style={{ height: 'calc(100dvh - 170px)' }}
    >
      <div className="flex justify-between items-center text-base mb-8">
        <div className="text-text-primary">Search Settings</div>
        <div onClick={() => setOpen(false)}>
          <PanelRightClose
            size={16}
            className="text-text-primary cursor-pointer"
          />
        </div>
      </div>
      <div
        style={{ height: 'calc(100dvh - 270px)' }}
        className="overflow-y-auto scrollbar-auto p-1 text-text-secondary"
      >
        <Form {...formMethods}>
          <form
            onSubmit={formMethods.handleSubmit((data) => console.log(data))}
            className="space-y-6"
          >
            {/* Name */}
            <FormField
              control={formMethods.control}
              name="name"
              rules={{ required: 'Name is required' }}
              render={({ field }) => (
                <FormItem>
                  <FormLabel>
                    <span className="text-destructive mr-1"> *</span>Name
                  </FormLabel>
                  <FormControl>
                    <Input placeholder="Name" {...field} />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />

            {/* Avatar */}
            <FormField
              control={formMethods.control}
              name="avatar"
              render={() => (
                <FormItem>
                  <FormLabel>Avatar</FormLabel>
                  <FormControl>
                    <div className="relative group">
                      {!avatarBase64Str ? (
                        <div className="w-[64px] h-[64px] grid place-content-center border border-dashed	rounded-md">
                          <div className="flex flex-col items-center">
                            <Upload />
                            <p>{t('common.upload')}</p>
                          </div>
                        </div>
                      ) : (
                        <div className="w-[64px] h-[64px] relative grid place-content-center">
                          <RAGFlowAvatar
                            avatar={avatarBase64Str}
                            name={data.name}
                            className="w-[64px] h-[64px] rounded-md block"
                          />
                          <div className="absolute inset-0 bg-[#000]/20 group-hover:bg-[#000]/60">
                            <Pencil
                              size={20}
                              className="absolute right-2 bottom-0 opacity-50 hidden group-hover:block"
                            />
                          </div>
                        </div>
                      )}
                      <Input
                        placeholder=""
                        // {...field}
                        type="file"
                        title=""
                        accept="image/*"
                        className="absolute top-0 left-0 w-full h-full opacity-0 cursor-pointer"
                        onChange={(ev) => {
                          const file = ev.target?.files?.[0];
                          if (
                            /\.(jpg|jpeg|png|webp|bmp)$/i.test(file?.name ?? '')
                          ) {
                            setAvatarFile(file!);
                          }
                          ev.target.value = '';
                        }}
                      />
                    </div>
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />

            {/* Description */}
            <FormField
              control={formMethods.control}
              name="description"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>Description</FormLabel>
                  <FormControl>
                    <Input
                      placeholder="Description"
                      {...field}
                      defaultValue="You are an intelligent assistant."
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />

            {/* Datasets */}
            <FormField
              control={formMethods.control}
              name="datasets"
              rules={{ required: 'Datasets is required' }}
              render={({ field }) => (
                <FormItem>
                  <FormLabel>
                    <span className="text-destructive mr-1"> *</span>Datasets
                  </FormLabel>
                  <FormControl>
                    <MultiSelect
                      options={datasetList}
                      onValueChange={(value) => {
                        handleDatasetSelectChange(value, field.onChange);
                      }}
                      showSelectAll={false}
                      placeholder={t('chat.knowledgeBasesMessage')}
                      variant="inverted"
                      maxCount={10}
                      {...field}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />

            {/* Keyword Similarity Weight */}
            <FormField
              control={formMethods.control}
              name="keywordSimilarityWeight"
              render={({ field }) => (
                <FormItem className="flex flex-col">
                  <FormLabel>Keyword Similarity Weight</FormLabel>
                  <FormControl>
                    <div className="flex justify-between items-center">
                      <SingleFormSlider
                        max={100}
                        step={1}
                        value={field.value as number}
                        onChange={(values) => field.onChange(values)}
                      ></SingleFormSlider>
                      <Label className="w-10 h-6 bg-bg-card flex justify-center items-center rounded-lg ml-20">
                        {field.value}
                      </Label>
                    </div>
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />

            {/* Rerank Model */}
            <FormField
              control={formMethods.control}
              name="rerankModel"
              render={({ field }) => (
                <FormItem className="flex flex-row items-start space-x-3 space-y-0">
                  <FormControl>
                    <Switch
                      checked={field.value}
                      onCheckedChange={field.onChange}
                    />
                  </FormControl>
                  <FormLabel>Rerank Model</FormLabel>
                </FormItem>
              )}
            />

            {/* AI Summary */}
            <FormField
              control={formMethods.control}
              name="aiSummary"
              render={({ field }) => (
                <FormItem className="flex flex-row items-start space-x-3 space-y-0">
                  <FormControl>
                    <Switch
                      checked={field.value}
                      onCheckedChange={field.onChange}
                    />
                  </FormControl>
                  <FormLabel>AI Summary</FormLabel>
                  <Label className="text-sm text-muted-foreground">
                    默认不打开
                  </Label>
                </FormItem>
              )}
            />

            {/* Top K */}
            <FormField
              control={formMethods.control}
              name="topK"
              render={({ field }) => (
                <FormItem className="flex flex-row items-start space-x-3 space-y-0">
                  <FormControl>
                    <Switch
                      checked={field.value}
                      onCheckedChange={field.onChange}
                    />
                  </FormControl>
                  <FormLabel>Top K</FormLabel>
                </FormItem>
              )}
            />

            {/* Search Method */}
            <FormField
              control={formMethods.control}
              name="searchMethod"
              rules={{ required: 'Search Method is required' }}
              render={({ field }) => (
                <FormItem>
                  <FormLabel>
                    <span className="text-destructive mr-1"> *</span>Search
                    Method
                  </FormLabel>
                  <FormControl>
                    <Select
                      onValueChange={field.onChange}
                      defaultValue={field.value}
                    >
                      <SelectTrigger>
                        <SelectValue placeholder="Select search method..." />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="method1">Method 1</SelectItem>
                        <SelectItem value="method2">Method 2</SelectItem>
                      </SelectContent>
                    </Select>
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />

            {/* Model */}
            <FormField
              control={formMethods.control}
              name="model"
              rules={{ required: 'Model is required' }}
              render={({ field }) => (
                <FormItem>
                  <FormLabel>
                    <span className="text-destructive mr-1"> *</span>Model
                  </FormLabel>
                  <FormControl>
                    <Select
                      onValueChange={field.onChange}
                      defaultValue={field.value}
                    >
                      <SelectTrigger>
                        <SelectValue placeholder="Select model..." />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="model1">Model 1</SelectItem>
                        <SelectItem value="model2">Model 2</SelectItem>
                      </SelectContent>
                    </Select>
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />

            {/* Feature Controls */}
            <FormField
              control={formMethods.control}
              name="enableWebSearch"
              render={({ field }) => (
                <FormItem className="flex flex-row items-start space-x-3 space-y-0">
                  <FormControl>
                    <Switch
                      checked={field.value}
                      onCheckedChange={field.onChange}
                    />
                  </FormControl>
                  <FormLabel>Enable Web Search</FormLabel>
                </FormItem>
              )}
            />

            <FormField
              control={formMethods.control}
              name="enableRelatedSearch"
              render={({ field }) => (
                <FormItem className="flex flex-row items-start space-x-3 space-y-0">
                  <FormControl>
                    <Switch
                      checked={field.value}
                      onCheckedChange={field.onChange}
                    />
                  </FormControl>
                  <FormLabel>Enable Related Search</FormLabel>
                </FormItem>
              )}
            />

            <FormField
              control={formMethods.control}
              name="showQueryMindmap"
              render={({ field }) => (
                <FormItem className="flex flex-row items-start space-x-3 space-y-0">
                  <FormControl>
                    <Switch
                      checked={field.value}
                      onCheckedChange={field.onChange}
                    />
                  </FormControl>
                  <FormLabel>Show Query Mindmap</FormLabel>
                </FormItem>
              )}
            />
            {/* Submit Button */}
            <div className="flex justify-end">
              <Button type="submit">Confirm</Button>
            </div>
          </form>
        </Form>
      </div>
    </div>
  );
};

export { SearchSetting };
