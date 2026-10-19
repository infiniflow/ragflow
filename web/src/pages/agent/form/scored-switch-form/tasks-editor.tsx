import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { BlurTextarea } from '@/components/ui/textarea';
import { useTranslate } from '@/hooks/common-hooks';
import { Plus, Trash2 } from 'lucide-react';
import { useFormContext, useWatch } from 'react-hook-form';
import { v4 as uuid } from 'uuid';

export type ScoredSwitchTask = {
  id: string;
  type: 'noul' | 'score' | 'box';
  instructions: string;
  true_text?: string;
  false_text?: string;
  levels?: string[];
};

export function ScoredSwitchTasks({
  allowYesNo,
  allowLevels,
  allowBox,
}: {
  allowYesNo: boolean;
  allowLevels: boolean;
  allowBox: boolean;
}) {
  const { t } = useTranslate('flow');
  const form = useFormContext();
  const tasks = (useWatch({ control: form.control, name: 'tasks' }) ??
    []) as ScoredSwitchTask[];
  const visible = tasks.filter((task) => {
    if (task.type === 'noul') return allowYesNo;
    if (task.type === 'score') return allowLevels;
    if (task.type === 'box') return allowBox;
    return false;
  });
  if (!allowYesNo && !allowLevels && !allowBox) {
    return null;
  }

  const write = (next: ScoredSwitchTask[]) => {
    form.setValue('tasks', next, { shouldDirty: true, shouldValidate: true });
  };

  const add = (type: ScoredSwitchTask['type']) => {
    const id = `${type}_${uuid()}`;
    const task: ScoredSwitchTask = {
      id,
      type,
      instructions: '',
      levels: type === 'score' ? ['', ''] : undefined,
    };
    write([...tasks, task]);
  };

  return (
    <div className="space-y-3">
      {visible.map((task) => (
        <div key={task.id} className="space-y-2 rounded-md border border-border-button p-3">
          <div className="flex items-center justify-between gap-3">
            <span className="text-sm text-text-secondary">
              {task.type === 'noul'
                ? t('scoredSwitchYesNo')
                : task.type === 'score'
                  ? t('scoredSwitchLevels')
                  : t('scoredSwitchBox')}
            </span>
            <Button
              type="button"
              variant="ghost"
              className="h-8 px-2 text-state-error"
              onClick={() => write(tasks.filter((item) => item.id !== task.id))}
            >
              <Trash2 className="size-4" />
            </Button>
          </div>
          <BlurTextarea
            rows={2}
            value={task.instructions}
            placeholder={t('scoredSwitchQuestionInstructions')}
            onChange={(value) =>
              write(
                tasks.map((item) =>
                  item.id === task.id
                    ? { ...item, instructions: value ?? '' }
                    : item,
                ),
              )
            }
          />
          {task.type === 'noul' && (
            <>
              <Input
                value={task.true_text ?? ''}
                placeholder={t('scoredSwitchTrueMeans')}
                onChange={(event) =>
                  write(
                    tasks.map((item) =>
                      item.id === task.id
                        ? { ...item, true_text: event.target.value }
                        : item,
                    ),
                  )
                }
              />
              <Input
                value={task.false_text ?? ''}
                placeholder={t('scoredSwitchFalseMeans')}
                onChange={(event) =>
                  write(
                    tasks.map((item) =>
                      item.id === task.id
                        ? { ...item, false_text: event.target.value }
                        : item,
                    ),
                  )
                }
              />
            </>
          )}
          {task.type === 'score' && (
            <div className="space-y-2">
              {(task.levels ?? []).map((level, index) => (
                <Input
                  key={`${task.id}-${index}`}
                  value={level}
                  placeholder={t('scoredSwitchLevelName')}
                  onChange={(event) => {
                    const levels = [...(task.levels ?? [])];
                    levels[index] = event.target.value;
                    write(
                      tasks.map((item) =>
                        item.id === task.id ? { ...item, levels } : item,
                      ),
                    );
                  }}
                />
              ))}
              <Button
                type="button"
                variant="outline"
                onClick={() =>
                  write(
                    tasks.map((item) =>
                      item.id === task.id
                        ? { ...item, levels: [...(item.levels ?? []), ''] }
                        : item,
                    ),
                  )
                }
              >
                {t('scoredSwitchAddLevel')}
              </Button>
            </div>
          )}
        </div>
      ))}
      <div className="flex flex-wrap gap-2">
        {allowYesNo && (
          <Button type="button" variant="outline" onClick={() => add('noul')}>
            <Plus className="mr-1 size-4" />
            {t('scoredSwitchYesNo')}
          </Button>
        )}
        {allowLevels && (
          <Button type="button" variant="outline" onClick={() => add('score')}>
            <Plus className="mr-1 size-4" />
            {t('scoredSwitchLevels')}
          </Button>
        )}
        {allowBox && (
          <Button type="button" variant="outline" onClick={() => add('box')}>
            <Plus className="mr-1 size-4" />
            {t('scoredSwitchBox')}
          </Button>
        )}
      </div>
    </div>
  );
}
