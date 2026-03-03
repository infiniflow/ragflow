import { Button } from '@/components/ui/button';
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
  CommandSeparator,
} from '@/components/ui/command';
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover';
import { Separator } from '@/components/ui/separator';
import { cn } from '@/lib/utils';
import { CheckIcon, ChevronDown, FileText, XIcon } from 'lucide-react';
import { useState } from 'react';

interface DocumentSelectorProps {
  documents: Array<{ doc_id: string; doc_name: string }>;
  selectedDocumentIds: string[];
  onSelectionChange: (documentIds: string[]) => void;
  disabled?: boolean;
}

export function DocumentSelector({
  documents,
  selectedDocumentIds,
  onSelectionChange,
  disabled = false,
}: DocumentSelectorProps) {
  const [isPopoverOpen, setIsPopoverOpen] = useState(false);

  const handleTogglePopover = () => {
    if (!disabled) {
      setIsPopoverOpen((prev) => !prev);
    }
  };

  const handleClear = () => {
    onSelectionChange([]);
  };

  const toggleOption = (docId: string) => {
    const newSelectedValues = selectedDocumentIds.includes(docId)
      ? selectedDocumentIds.filter((id) => id !== docId)
      : [...selectedDocumentIds, docId];
    onSelectionChange(newSelectedValues);
  };

  if (documents.length === 0) {
    return null;
  }

  return (
    <Popover open={isPopoverOpen} onOpenChange={setIsPopoverOpen}>
      <PopoverTrigger asChild>
        <Button
          onClick={handleTogglePopover}
          disabled={disabled}
          variant="outline"
          className={cn(
            'flex h-9 px-3 rounded-lg text-sm border items-center justify-between bg-inherit hover:bg-muted',
          )}
        >
          <div className="flex items-center gap-2">
            <FileText className="size-4" />
            <span>
              {selectedDocumentIds.length > 0
                ? `已选 ${selectedDocumentIds.length} 个文档`
                : '选择文档'}
            </span>
          </div>
          <div className="flex items-center">
            {selectedDocumentIds.length > 0 && (
              <>
                <XIcon
                  className="h-4 mx-1 cursor-pointer text-muted-foreground hover:text-foreground"
                  onClick={(event) => {
                    event.stopPropagation();
                    handleClear();
                  }}
                />
                <Separator orientation="vertical" className="h-4 mx-1" />
              </>
            )}
            <ChevronDown className="h-4 text-muted-foreground" />
          </div>
        </Button>
      </PopoverTrigger>
      <PopoverContent
        className="w-80 p-0"
        align="start"
        onEscapeKeyDown={() => setIsPopoverOpen(false)}
      >
        <Command>
          <CommandInput placeholder="搜索文档..." />
          <CommandList>
            <CommandEmpty>未找到文档</CommandEmpty>
            <CommandGroup>
              {documents.map((doc) => {
                const isSelected = selectedDocumentIds.includes(doc.doc_id);
                return (
                  <CommandItem
                    key={doc.doc_id}
                    onSelect={() => toggleOption(doc.doc_id)}
                    className="cursor-pointer"
                  >
                    <div
                      className={cn(
                        'mr-2 flex h-4 w-4 items-center justify-center rounded-sm border border-primary',
                        isSelected
                          ? 'bg-primary text-primary-foreground'
                          : 'opacity-50 [&_svg]:invisible',
                      )}
                    >
                      <CheckIcon className="h-4 w-4" />
                    </div>
                    <FileText className="mr-2 h-4 w-4 text-muted-foreground" />
                    <span className="flex-1 truncate">{doc.doc_name}</span>
                  </CommandItem>
                );
              })}
            </CommandGroup>
            <CommandSeparator />
            <CommandGroup>
              <div className="flex items-center justify-between">
                {selectedDocumentIds.length > 0 && (
                  <>
                    <CommandItem
                      onSelect={handleClear}
                      className="flex-1 justify-center cursor-pointer"
                    >
                      清空
                    </CommandItem>
                    <Separator orientation="vertical" className="h-6" />
                  </>
                )}
                <CommandItem
                  onSelect={() => setIsPopoverOpen(false)}
                  className="flex-1 justify-center cursor-pointer"
                >
                  关闭
                </CommandItem>
              </div>
            </CommandGroup>
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  );
}
