import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { Switch } from '@/components/ui/switch';
import { BlurTextarea } from '@/components/ui/textarea';
import { useTranslate } from '@/hooks/common-hooks';
import { Plus, Trash2 } from 'lucide-react';
import { useFormContext, useWatch } from 'react-hook-form';
import { v4 as uuid } from 'uuid';
import { ScoredSwitchTask, ScoredSwitchTaskType } from './tasks-editor';

const fieldTypes = [
  'text',
  'date',
  'time',
  'datetime',
  'duration',
  'number',
  'list',
  'estimate',
  'forecast',
] as const;

type FieldType = (typeof fieldTypes)[number];

const durationUnits = [
  'seconds',
  'minutes',
  'hours',
  'days',
  'weeks',
  'months',
  'years',
];

const forecastIntervals = [
  'step',
  'minute',
  'hour',
  'day',
  'week',
  'month',
  'quarter',
  'year',
];

function typeLabel(t: (key: string) => string, type: FieldType) {
  switch (type) {
    case 'text':
      return t('scoredSwitchFieldText');
    case 'date':
      return t('scoredSwitchFieldDate');
    case 'time':
      return t('scoredSwitchFieldTime');
    case 'datetime':
      return t('scoredSwitchFieldDateTime');
    case 'duration':
      return t('scoredSwitchFieldDuration');
    case 'number':
      return t('scoredSwitchFieldNumber');
    case 'list':
      return t('scoredSwitchFieldList');
    case 'estimate':
      return t('scoredSwitchFieldEstimate');
    case 'forecast':
      return t('scoredSwitchFieldForecast');
    default:
      return type;
  }
}

export function ScoredSwitchFields({
  allow,
}: {
  allow: Record<FieldType, boolean>;
}) {
  const { t } = useTranslate('flow');
  const form = useFormContext();
  const tasks = (useWatch({ control: form.control, name: 'tasks' }) ??
    []) as ScoredSwitchTask[];
  const enabled = fieldTypes.filter((type) => allow[type]);
  const visible = tasks.filter(
    (task) =>
      fieldTypes.includes(task.type as FieldType) &&
      allow[task.type as FieldType],
  );
  const taskError = form.formState.errors.tasks;
  const taskMessage =
    taskError && !Array.isArray(taskError) && typeof taskError.message === 'string'
      ? taskError.message
      : '';
  if (enabled.length === 0) {
    return null;
  }

  const write = (next: ScoredSwitchTask[]) => {
    form.setValue('tasks', next, { shouldDirty: true, shouldValidate: true });
  };

  const patch = (id: string, change: Partial<ScoredSwitchTask>) => {
    write(tasks.map((item) => (item.id === id ? { ...item, ...change } : item)));
  };

  const add = (type: FieldType) => {
    const suffix = uuid().replace(/-/g, '').slice(0, 8);
    const task: ScoredSwitchTask = {
      id: `${type}_${suffix}`,
      type: type as ScoredSwitchTaskType,
      instructions: '',
      items: type === 'list' ? 'text' : undefined,
      scale: type === 'estimate' || type === 'forecast' ? 'linear' : undefined,
      interval: type === 'forecast' ? 'day' : undefined,
      horizon: type === 'forecast' ? 1 : undefined,
    };
    write([...tasks, task]);
  };

  return (
    <div className="space-y-3">
      {visible.map((task) => (
        <div
          key={task.id}
          className="space-y-2 rounded-md border border-border-button p-3"
        >
          <div className="flex items-center justify-between gap-3">
            <span className="text-sm text-text-secondary">
              {typeLabel(t, task.type as FieldType)}
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
          <Input
            value={task.id}
            placeholder={t('scoredSwitchFieldName')}
            onChange={(event) => patch(task.id, { id: event.target.value })}
          />
          <BlurTextarea
            rows={2}
            value={task.instructions}
            placeholder={t('scoredSwitchQuestionInstructions')}
            onChange={(value) => patch(task.id, { instructions: value ?? '' })}
          />
          {(task.type === 'number' || task.type === 'duration') && (
            <div className="grid grid-cols-2 gap-2">
              {task.type === 'number' ? (
                <Input
                  value={task.unit ?? ''}
                  placeholder={t('scoredSwitchFieldUnit')}
                  onChange={(event) => patch(task.id, { unit: event.target.value })}
                />
              ) : (
                <Select
                  value={task.unit || 'seconds'}
                  onValueChange={(unit) => patch(task.id, { unit })}
                >
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {durationUnits.map((unit) => (
                      <SelectItem key={unit} value={unit}>
                        {t(`scoredSwitchUnit_${unit}`)}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              )}
              <Input
                type="number"
                min={0}
                max={6}
                value={task.decimals ?? ''}
                placeholder={t('scoredSwitchFieldDecimals')}
                onChange={(event) =>
                  patch(task.id, {
                    decimals:
                      event.target.value === ''
                        ? ''
                        : Number(event.target.value),
                  })
                }
              />
            </div>
          )}
          {(task.type === 'number' || task.type === 'duration') && (
            <label className="flex items-center justify-between gap-3 text-sm">
              {t('scoredSwitchFieldInteger')}
              <Switch
                checked={task.integer === true}
                onCheckedChange={(integer) => patch(task.id, { integer })}
              />
            </label>
          )}
          {task.type === 'list' && (
            <Select
              value={task.items || 'text'}
              onValueChange={(items) => patch(task.id, { items })}
            >
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="text">{t('scoredSwitchFieldText')}</SelectItem>
                <SelectItem value="number">{t('scoredSwitchFieldNumber')}</SelectItem>
                <SelectItem value="date">{t('scoredSwitchFieldDate')}</SelectItem>
              </SelectContent>
            </Select>
          )}
          {(task.type === 'estimate' || task.type === 'forecast') && (
            <div className="grid grid-cols-2 gap-2">
              <Input
                value={task.min ?? ''}
                placeholder={t('scoredSwitchFieldMin')}
                onChange={(event) => patch(task.id, { min: event.target.value })}
              />
              <Input
                value={task.max ?? ''}
                placeholder={t('scoredSwitchFieldMax')}
                onChange={(event) => patch(task.id, { max: event.target.value })}
              />
              <Input
                value={task.step ?? ''}
                placeholder={t('scoredSwitchFieldStep')}
                onChange={(event) => patch(task.id, { step: event.target.value })}
              />
              <Select
                value={task.scale || 'linear'}
                onValueChange={(scale) => patch(task.id, { scale })}
              >
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="linear">{t('scoredSwitchScaleLinear')}</SelectItem>
                  <SelectItem value="log">{t('scoredSwitchScaleLog')}</SelectItem>
                </SelectContent>
              </Select>
            </div>
          )}
          {task.type === 'forecast' && (
            <div className="grid grid-cols-2 gap-2">
              <Input
                type="number"
                min={1}
                max={64}
                value={task.horizon ?? ''}
                placeholder={t('scoredSwitchFieldHorizon')}
                onChange={(event) =>
                  patch(task.id, {
                    horizon:
                      event.target.value === ''
                        ? ''
                        : Number(event.target.value),
                  })
                }
              />
              <Select
                value={task.interval || 'day'}
                onValueChange={(interval) => patch(task.id, { interval })}
              >
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {forecastIntervals.map((interval) => (
                    <SelectItem key={interval} value={interval}>
                      {t(`scoredSwitchInterval_${interval}`)}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          )}
        </div>
      ))}
      <div className="flex flex-wrap gap-2">
        {enabled.map((type) => (
          <Button key={type} type="button" variant="outline" onClick={() => add(type)}>
            <Plus className="mr-1 size-4" />
            {typeLabel(t, type)}
          </Button>
        ))}
      </div>
      {taskMessage && (
        <p className="text-sm text-state-error">{taskMessage}</p>
      )}
    </div>
  );
}
