import { KeyInput } from '@/components/key-input';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip';
import { CirclePlus, HelpCircle, Info } from 'lucide-react';
import { useId, useState, type FC, type FormEvent } from 'react';
import { useTranslation } from '../../hooks/use-translation';
import type { NewField, SchemaType } from '../../types/json-schema';
import { KeyInputProps } from './interface';
import SchemaTypeSelector from './schema-type-selector';

interface AddFieldButtonProps {
  onAddField: (field: NewField) => void;
  variant?: 'primary' | 'secondary';
}

const AddFieldButton: FC<AddFieldButtonProps & KeyInputProps> = ({
  onAddField,
  variant = 'primary',
  pattern,
}) => {
  const [dialogOpen, setDialogOpen] = useState(false);
  const [fieldName, setFieldName] = useState('');
  const [fieldType, setFieldType] = useState<SchemaType>('string');
  const [fieldDesc, setFieldDesc] = useState('');
  const [fieldRequired, setFieldRequired] = useState(false);
  const fieldNameId = useId();
  const fieldDescId = useId();
  const fieldRequiredId = useId();
  const fieldTypeId = useId();

  const t = useTranslation();

  const handleSubmit = (e: FormEvent) => {
    e.preventDefault();
    if (!fieldName.trim()) return;

    onAddField({
      name: fieldName,
      type: fieldType,
      description: fieldDesc,
      required: fieldRequired,
    });

    setFieldName('');
    setFieldType('string');
    setFieldDesc('');
    setFieldRequired(false);
    setDialogOpen(false);
  };

  return (
    <>
      <Button
        type="button"
        onClick={() => setDialogOpen(true)}
        variant={variant === 'primary' ? 'default' : 'outline'}
        size="sm"
        className="flex items-center gap-1.5 group"
      >
        <CirclePlus
          size={16}
          className="group-hover:scale-110 transition-transform"
        />
        <span>{t.fieldAddNewButton}</span>
      </Button>

      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent className="md:max-w-[1200px] max-h-[85vh] w-[95vw] p-4 sm:p-6 jsonjoy">
          <DialogHeader className="mb-4">
            <DialogTitle className="text-xl flex flex-wrap items-center gap-2">
              {t.fieldAddNewLabel}
              <Badge variant="secondary" className="text-xs">
                {t.fieldAddNewBadge}
              </Badge>
            </DialogTitle>
            <DialogDescription className="text-sm">
              {t.fieldAddNewDescription}
            </DialogDescription>
          </DialogHeader>

          <form onSubmit={handleSubmit} className="space-y-6">
            <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
              <div className="space-y-4 min-w-[280px]">
                <div>
                  <div className="flex flex-wrap items-center gap-2 mb-1.5">
                    <label
                      htmlFor={fieldNameId}
                      className="text-sm font-medium"
                    >
                      {t.fieldNameLabel}
                    </label>
                    <TooltipProvider>
                      <Tooltip>
                        <TooltipTrigger asChild>
                          <Info className="h-4 w-4 text-muted-foreground shrink-0" />
                        </TooltipTrigger>
                        <TooltipContent className="max-w-[90vw]">
                          <p>{t.fieldNameTooltip}</p>
                        </TooltipContent>
                      </Tooltip>
                    </TooltipProvider>
                  </div>
                  <KeyInput
                    id={fieldNameId}
                    value={fieldName}
                    onChange={setFieldName}
                    placeholder={t.fieldNamePlaceholder}
                    className="font-mono text-sm w-full"
                    required
                    searchValue={pattern}
                  />
                </div>

                <div>
                  <div className="flex flex-wrap items-center gap-2 mb-1.5">
                    <label
                      htmlFor={fieldDescId}
                      className="text-sm font-medium"
                    >
                      {t.fieldDescription}
                    </label>
                    <TooltipProvider>
                      <Tooltip>
                        <TooltipTrigger asChild>
                          <Info className="h-4 w-4 text-muted-foreground shrink-0" />
                        </TooltipTrigger>
                        <TooltipContent className="max-w-[90vw]">
                          <p>{t.fieldDescriptionTooltip}</p>
                        </TooltipContent>
                      </Tooltip>
                    </TooltipProvider>
                  </div>
                  <Input
                    id={fieldDescId}
                    value={fieldDesc}
                    onChange={(e) => setFieldDesc(e.target.value)}
                    placeholder={t.fieldDescriptionPlaceholder}
                    className="text-sm w-full"
                  />
                </div>

                <div className="flex items-center gap-3 p-3 rounded-lg border bg-muted/50">
                  <input
                    type="checkbox"
                    id={fieldRequiredId}
                    checked={fieldRequired}
                    onChange={(e) => setFieldRequired(e.target.checked)}
                    className="rounded border-gray-300 shrink-0"
                  />
                  <label htmlFor={fieldRequiredId} className="text-sm">
                    {t.fieldRequiredLabel}
                  </label>
                </div>
              </div>

              <div className="space-y-4 min-w-[280px]">
                <div>
                  <div className="flex flex-wrap items-center gap-2 mb-1.5">
                    <label
                      htmlFor={fieldTypeId}
                      className="text-sm font-medium"
                    >
                      {t.fieldType}
                    </label>
                    <TooltipProvider>
                      <Tooltip>
                        <TooltipTrigger asChild>
                          <HelpCircle className="h-4 w-4 text-muted-foreground shrink-0" />
                        </TooltipTrigger>
                        <TooltipContent
                          side="left"
                          className="w-72 max-w-[90vw]"
                        >
                          <div className="grid grid-cols-2 gap-x-4 gap-y-1 text-xs">
                            <div>• {t.fieldTypeTooltipString}</div>
                            <div>• {t.fieldTypeTooltipNumber}</div>
                            <div>• {t.fieldTypeTooltipBoolean}</div>
                            <div>• {t.fieldTypeTooltipObject}</div>
                            <div className="col-span-2">
                              • {t.fieldTypeTooltipArray}
                            </div>
                          </div>
                        </TooltipContent>
                      </Tooltip>
                    </TooltipProvider>
                  </div>
                  <SchemaTypeSelector
                    id={fieldTypeId}
                    value={fieldType}
                    onChange={setFieldType}
                  />
                </div>

                <div className="rounded-lg border bg-muted/50 p-3 hidden md:block">
                  <p className="text-xs font-medium mb-2">
                    {t.fieldTypeExample}
                  </p>
                  <code className="text-sm bg-background/80 p-2 rounded block overflow-x-auto">
                    {fieldType === 'string' && '"example"'}
                    {fieldType === 'number' && '42'}
                    {fieldType === 'boolean' && 'true'}
                    {fieldType === 'object' && '{ "key": "value" }'}
                    {fieldType === 'array' && '["item1", "item2"]'}
                  </code>
                </div>
              </div>
            </div>

            <DialogFooter className="mt-6 gap-2 flex-wrap">
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={() => setDialogOpen(false)}
              >
                {t.fieldAddNewCancel}
              </Button>
              <Button type="submit" size="sm">
                {t.fieldAddNewConfirm}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </>
  );
};

export default AddFieldButton;
