import { Calendar } from '@/components/originui/calendar';
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from '@/components/ui/alert-dialog';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
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
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { Separator } from '@/components/ui/separator';
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet';
import { Switch } from '@/components/ui/switch';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import { Textarea } from '@/components/ui/textarea';
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip';
import { useTranslate } from '@/hooks/common-hooks';
import {
  useCreateSchedule,
  useDeleteSchedule,
  useFetchFrequencyOptions,
  useFetchScheduleHistory,
  useFetchSchedules,
  useFetchScheduleStats,
  useToggleSchedule,
  useUpdateSchedule,
} from '@/hooks/schedule-hooks';
import {
  ICreateScheduleRequest,
  ISchedule,
  IScheduleRun,
  IScheduleStats,
} from '@/interfaces/database/schedule';
import { cn } from '@/lib/utils';
import {
  CheckCircleOutlined,
  ClockCircleOutlined,
  CloseCircleOutlined,
  DeleteOutlined,
  EditOutlined,
  HistoryOutlined,
  ReloadOutlined,
} from '@ant-design/icons';
import { zodResolver } from '@hookform/resolvers/zod';
import dayjs from 'dayjs';
import timezone from 'dayjs/plugin/timezone';
import utc from 'dayjs/plugin/utc';
import { CalendarIcon, Clock, Loader2 } from 'lucide-react';
import React, { useCallback, useState } from 'react';
import { useForm } from 'react-hook-form';
import * as z from 'zod';

// Configure dayjs plugins
dayjs.extend(utc);
dayjs.extend(timezone);

const scheduleFormSchema = z.object({
  name: z.string().min(1, 'Name is required'),
  description: z.string().optional(),
  frequency_type: z.string().min(1, 'Frequency type is required'),
  execute_time: z.date().optional(),
  execute_date: z.date().optional(),
  days_of_week: z.array(z.number()).optional(),
  day_of_month: z.number().optional(),
});

interface ScheduleFormModalProps {
  visible: boolean;
  onCancel: () => void;
  onSave: () => void;
  editingSchedule: ISchedule | null;
  canvasId: string;
  loading: boolean;
}

function ScheduleFormModal({
  visible,
  onCancel,
  onSave,
  editingSchedule,
  canvasId,
  loading,
}: ScheduleFormModalProps) {
  const { t } = useTranslate('flow');
  const [timePickerOpen, setTimePickerOpen] = useState(false);
  const [datePickerOpen, setDatePickerOpen] = useState(false);

  const { data: frequencyOptions, loading: loadingOptions } =
    useFetchFrequencyOptions();
  const { createSchedule, loading: creating } = useCreateSchedule();
  const { updateSchedule, loading: updating } = useUpdateSchedule();

  const form = useForm<z.infer<typeof scheduleFormSchema>>({
    resolver: zodResolver(scheduleFormSchema),
    defaultValues: {
      name: '',
      description: '',
      frequency_type: 'once',
      execute_time: dayjs().add(1, 'hour').toDate(),
      execute_date: dayjs().add(1, 'hour').toDate(),
      days_of_week: [],
      day_of_month: undefined,
    },
  });

  const frequencyType = form.watch('frequency_type');

  const getRequiredFields = useCallback(() => {
    if (!frequencyOptions?.frequency_types || !frequencyType) return [];

    const option = frequencyOptions.frequency_types.find(
      (type) => type.value === frequencyType,
    );
    return option?.required_fields || [];
  }, [frequencyOptions, frequencyType]);

  const handleSave = useCallback(
    async (values: z.infer<typeof scheduleFormSchema>) => {
      try {





        // Ensure frequency_type is always present - get from form if missing
        const formFrequencyType = form.getValues('frequency_type');
        if (!values.frequency_type && !formFrequencyType) {
          form.setError('frequency_type', {
            message: 'Frequency type is required',
          });
          return;
        }

        // Use the value from form state if not in values
        const finalFrequencyType = values.frequency_type || formFrequencyType;

        const payload: ICreateScheduleRequest = {
          canvas_id: canvasId,
          name: values.name.trim(),
          description: values.description?.trim() || '',
          frequency_type: finalFrequencyType,
        };

        // Get required fields for current frequency type
        const currentRequiredFields = getRequiredFields();

        // Handle time conversion
        if (
          currentRequiredFields.includes('execute_time') &&
          values.execute_time
        ) {
          payload.execute_time = dayjs(values.execute_time).format('HH:mm:ss');
        }

        // Handle date conversion
        if (
          currentRequiredFields.includes('execute_date') &&
          values.execute_date
        ) {
          payload.execute_date = dayjs(values.execute_date).toISOString();
        }

        // Handle days of week
        if (
          currentRequiredFields.includes('days_of_week') &&
          values.days_of_week &&
          values.days_of_week.length > 0
        ) {
          payload.days_of_week = values.days_of_week;
        }

        // Handle day of month
        if (
          currentRequiredFields.includes('day_of_month') &&
          values.day_of_month
        ) {
          payload.day_of_month = values.day_of_month;
        }

        console.log('Final payload:', payload);

        if (editingSchedule) {
          const updatePayload = { id: editingSchedule.id, ...payload };
          console.log('Update payload:', updatePayload);
          await updateSchedule(updatePayload);
        } else {
          await createSchedule(payload);
        }

        form.reset();
        onSave();
      } catch (error) {
        console.error('Save failed:', error);
      }
    },
    [
      canvasId,
      editingSchedule,
      createSchedule,
      updateSchedule,
      onSave,
      form,
      getRequiredFields,
    ],
  );
  // Set form values when editing schedule changes
  React.useEffect(() => {
    console.log('=== FORM EFFECT START ===');
    console.log('visible:', visible);
    console.log('editingSchedule:', editingSchedule);
    console.log(
      'frequencyOptions loaded:',
      !!frequencyOptions?.frequency_types,
    );

    if (visible && editingSchedule && frequencyOptions?.frequency_types) {
      console.log('Setting form values for editing:', editingSchedule);

      const formData: any = {
        name: editingSchedule.name || '',
        description: editingSchedule.description || '',
        frequency_type: editingSchedule.frequency_type || 'once',
        days_of_week: editingSchedule.days_of_week || [],
        day_of_month: editingSchedule.day_of_month || undefined,
      };

      // Handle time conversion
      if (editingSchedule.execute_time) {
        try {
          const timeStr = editingSchedule.execute_time;
          const timeParts = timeStr.split(':');
          const hours = parseInt(timeParts[0], 10);
          const minutes = parseInt(timeParts[1], 10);
          const seconds = parseInt(timeParts[2] || '0', 10);

          formData.execute_time = dayjs()
            .hour(hours)
            .minute(minutes)
            .second(seconds)
            .toDate();
        } catch (error) {
          console.warn(
            'Failed to parse execute_time:',
            editingSchedule.execute_time,
          );
          formData.execute_time = dayjs().toDate();
        }
      }

      // Handle date conversion
      if (editingSchedule.execute_date) {
        try {
          formData.execute_date = dayjs(editingSchedule.execute_date).toDate();
        } catch (error) {
          console.warn(
            'Failed to parse execute_date:',
            editingSchedule.execute_date,
          );
          formData.execute_date = dayjs().toDate();
        }
      }

      console.log('Form data to set:', formData);

      // Reset form with proper values
      form.reset(formData);

      // Trigger validation after form reset
      setTimeout(() => {
        console.log('Form values after reset:', form.getValues());
        console.log(
          'frequency_type after reset:',
          form.getValues('frequency_type'),
        );
        form.trigger();
      }, 200);
    } else if (visible && !editingSchedule) {
      // Set default values for new schedule
      const defaultTime = dayjs().add(1, 'hour').toDate();
      const defaultData = {
        name: '',
        description: '',
        frequency_type: 'once',
        execute_time: defaultTime,
        execute_date: defaultTime,
        days_of_week: [],
        day_of_month: undefined,
      };

      console.log('Setting default form data:', defaultData);
      form.reset(defaultData);
      setTimeout(() => {
        form.trigger();
      }, 100);
    }
  }, [visible, editingSchedule, form, frequencyOptions]);

  const requiredFields = getRequiredFields();

  return (
    <Dialog open={visible} onOpenChange={(open) => !open && onCancel()}>
      <DialogContent className="max-w-3xl">
        <DialogHeader>
          <DialogTitle>
            {editingSchedule ? t('schedule.edit') : t('schedule.create')}
          </DialogTitle>
        </DialogHeader>

        <Form {...form}>
          <form onSubmit={form.handleSubmit(handleSave)} className="space-y-6">
            <div className="grid grid-cols-2 gap-4">
              <FormField
                control={form.control}
                name="name"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('schedule.name')}</FormLabel>
                    <FormControl>
                      <Input
                        placeholder={t('schedule.namePlaceholder')}
                        {...field}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name="frequency_type"
                render={({ field }) => {
                  console.log('Frequency field render - value:', field.value);
                  return (
                    <FormItem>
                      <FormLabel>{t('schedule.frequency')}</FormLabel>
                      <Select
                        disabled={
                          loadingOptions || !frequencyOptions?.frequency_types
                        }
                        onValueChange={(value) => {
                          console.log('Frequency type changed to:', value);
                          field.onChange(value);
                        }}
                        value={field.value}
                      >
                        <FormControl>
                          <SelectTrigger>
                            <SelectValue
                              placeholder={t('schedule.frequencyPlaceholder')}
                            />
                          </SelectTrigger>
                        </FormControl>
                        <SelectContent>
                          {frequencyOptions?.frequency_types?.map((option) => (
                            <SelectItem key={option.value} value={option.value}>
                              <div className="py-1">
                                <div className="font-medium text-sm">
                                  {option.label}
                                </div>
                                <div className="text-xs text-muted-foreground mt-1 leading-tight">
                                  {option.description}
                                </div>
                              </div>
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                      <FormMessage />
                    </FormItem>
                  );
                }}
              />
            </div>

            <FormField
              control={form.control}
              name="description"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('schedule.description')}</FormLabel>
                  <FormControl>
                    <Textarea
                      placeholder={t('schedule.descriptionPlaceholder')}
                      className="resize-none"
                      rows={2}
                      {...field}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />

            {requiredFields.includes('execute_time') && (
              <div
                className={cn(
                  'grid gap-4',
                  requiredFields.includes('execute_date')
                    ? 'grid-cols-2'
                    : 'grid-cols-1',
                )}
              >
                <FormField
                  control={form.control}
                  name="execute_time"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('schedule.executeTime')}</FormLabel>
                      <Popover
                        open={timePickerOpen}
                        onOpenChange={setTimePickerOpen}
                      >
                        <PopoverTrigger asChild>
                          <FormControl>
                            <Button
                              variant="outline"
                              className={cn(
                                'w-full pl-3 text-left font-normal',
                                !field.value && 'text-muted-foreground',
                              )}
                            >
                              {field.value ? (
                                dayjs(field.value).format('HH:mm:ss')
                              ) : (
                                <span>
                                  {t('schedule.executeTimePlaceholder')}
                                </span>
                              )}
                              <Clock className="ml-auto h-4 w-4 opacity-50" />
                            </Button>
                          </FormControl>
                        </PopoverTrigger>
                        <PopoverContent className="w-auto p-0" align="start">
                          <div className="p-3">
                            <Input
                              type="time"
                              step="1"
                              value={
                                field.value
                                  ? dayjs(field.value).format('HH:mm:ss')
                                  : ''
                              }
                              onChange={(e) => {
                                if (e.target.value) {
                                  const [hours, minutes, seconds] =
                                    e.target.value.split(':');
                                  const newTime = dayjs()
                                    .hour(parseInt(hours))
                                    .minute(parseInt(minutes))
                                    .second(parseInt(seconds || '0'))
                                    .toDate();
                                  field.onChange(newTime);
                                }
                              }}
                            />
                          </div>
                        </PopoverContent>
                      </Popover>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                {requiredFields.includes('execute_date') && (
                  <FormField
                    control={form.control}
                    name="execute_date"
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>{t('schedule.executeDate')}</FormLabel>
                        <Popover
                          open={datePickerOpen}
                          onOpenChange={setDatePickerOpen}
                        >
                          <PopoverTrigger asChild>
                            <FormControl>
                              <Button
                                variant="outline"
                                className={cn(
                                  'w-full pl-3 text-left font-normal',
                                  !field.value && 'text-muted-foreground',
                                )}
                              >
                                {field.value ? (
                                  dayjs(field.value).format('YYYY-MM-DD')
                                ) : (
                                  <span>
                                    {t('schedule.executeDatePlaceholder')}
                                  </span>
                                )}
                                <CalendarIcon className="ml-auto h-4 w-4 opacity-50" />
                              </Button>
                            </FormControl>
                          </PopoverTrigger>
                          <PopoverContent className="w-auto p-0" align="start">
                            <Calendar
                              mode="single"
                              selected={field.value}
                              onSelect={(date) => {
                                field.onChange(date);
                                setDatePickerOpen(false);
                              }}
                              disabled={(date) =>
                                date < dayjs().startOf('day').toDate()
                              }
                              initialFocus
                            />
                          </PopoverContent>
                        </Popover>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                )}
              </div>
            )}

            {requiredFields.includes('days_of_week') && (
              <FormField
                control={form.control}
                name="days_of_week"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('schedule.daysOfWeek')}</FormLabel>
                    <div className="flex flex-wrap gap-2">
                      {frequencyOptions?.days_of_week?.map((day) => (
                        <Button
                          key={day.value}
                          type="button"
                          variant={
                            field.value?.includes(day.value)
                              ? 'default'
                              : 'outline'
                          }
                          size="sm"
                          onClick={() => {
                            const currentValues = field.value || [];
                            if (currentValues.includes(day.value)) {
                              field.onChange(
                                currentValues.filter((v) => v !== day.value),
                              );
                            } else {
                              field.onChange([...currentValues, day.value]);
                            }
                          }}
                        >
                          {day.label}
                        </Button>
                      ))}
                    </div>
                    <FormMessage />
                  </FormItem>
                )}
              />
            )}

            {requiredFields.includes('day_of_month') && (
              <FormField
                control={form.control}
                name="day_of_month"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('schedule.dayOfMonth')}</FormLabel>
                    <Select
                      onValueChange={(value) => field.onChange(parseInt(value))}
                      value={field.value?.toString()}
                    >
                      <FormControl>
                        <SelectTrigger>
                          <SelectValue
                            placeholder={t('schedule.dayOfMonthPlaceholder')}
                          />
                        </SelectTrigger>
                      </FormControl>
                      <SelectContent>
                        {Array.from({ length: 31 }, (_, i) => i + 1).map(
                          (day) => (
                            <SelectItem key={day} value={day.toString()}>
                              {day}
                            </SelectItem>
                          ),
                        )}
                      </SelectContent>
                    </Select>
                    <FormMessage />
                  </FormItem>
                )}
              />
            )}

            <DialogFooter>
              <Button type="button" variant="outline" onClick={onCancel}>
                {t('common.cancel')}
              </Button>
              <Button
                type="submit"
                disabled={creating || updating || loading || !frequencyOptions}
                onClick={() => {
                  console.log('Submit button clicked');
                  console.log('Current form values:', form.getValues());
                  console.log('Form is valid:', form.formState.isValid);
                  console.log('Form errors:', form.formState.errors);
                }}
              >
                {(creating || updating) && (
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                )}
                {editingSchedule ? t('common.update') : t('common.create')}
              </Button>
            </DialogFooter>
          </form>
        </Form>
      </DialogContent>
    </Dialog>
  );
}

interface ScheduleRunDrawerProps {
  visible: boolean;
  onClose: () => void;
  schedule: ISchedule | null;
}

function ScheduleRunDrawer({
  visible,
  onClose,
  schedule,
}: ScheduleRunDrawerProps) {
  const { t } = useTranslate('flow');

  const {
    stats,
    loading: loadingStats,
    refetch: refetchStats,
  } = useFetchScheduleStats(schedule?.id || '');
  const {
    history,
    loading: loadingHistory,
    refetch: refetchHistory,
  } = useFetchScheduleHistory(schedule?.id || '');

  const formatDateTime = useCallback((dateTime: Date) => {
    try {
      return dayjs(dateTime).tz(dayjs.tz.guess()).format('YYYY-MM-DD HH:mm:ss');
    } catch (error) {
      return '-';
    }
  }, []);

  const calculateDuration = useCallback((startTime?: Date, endTime?: Date) => {
    if (!endTime) return null;

    try {
      const start = dayjs(startTime);
      const end = dayjs(endTime);
      return end.diff(start, 'seconds');
    } catch (error) {
      return null;
    }
  }, []);

  const formatDuration = useCallback((duration: number | null) => {
    if (!duration || duration <= 0) return '-';

    const minutes = Math.floor(duration / 60);
    const seconds = Math.floor(duration % 60);

    if (minutes > 0) {
      return `${minutes}m ${seconds}s`;
    }
    return `${seconds}s`;
  }, []);

  const getStatusBadge = useCallback(
    (run: IScheduleRun) => {
      if (run.finished_at === null || run.finished_at === undefined) {
        return (
          <Badge variant="secondary" className="bg-blue-100 text-blue-800">
            <ClockCircleOutlined className="mr-1" />
            {t('schedule.running')}
          </Badge>
        );
      }

      if (run.success) {
        return (
          <Badge variant="secondary" className="bg-green-100 text-green-800">
            <CheckCircleOutlined className="mr-1" />
            {t('schedule.success')}
          </Badge>
        );
      }

      return (
        <Badge variant="destructive">
          <CloseCircleOutlined className="mr-1" />
          {t('schedule.failed')}
        </Badge>
      );
    },
    [t],
  );

  const handleRefresh = useCallback(() => {
    refetchStats();
    refetchHistory();
  }, [refetchStats, refetchHistory]);

  return (
    <Sheet open={visible} onOpenChange={onClose}>
      <SheetContent className="w-full max-w-4xl">
        <SheetHeader className="flex flex-row items-center justify-between">
          <SheetTitle>
            {t('schedule.runInfo')} - {schedule?.name}
          </SheetTitle>
          <Button
            variant="outline"
            size="sm"
            onClick={handleRefresh}
            disabled={loadingStats || loadingHistory}
          >
            {loadingStats || loadingHistory ? (
              <Loader2 className="h-4 w-4 animate-spin" />
            ) : (
              <ReloadOutlined className="h-4 w-4" />
            )}
            {t('common.refresh')}
          </Button>
        </SheetHeader>

        {schedule && (
          <div className="space-y-6 mt-6">
            {/* Stats Section */}
            <Card>
              <CardHeader>
                <CardTitle>{t('schedule.statistics')}</CardTitle>
              </CardHeader>
              <CardContent>
                {loadingStats ? (
                  <div className="flex justify-center">
                    <Loader2 className="h-6 w-6 animate-spin" />
                  </div>
                ) : (
                  <>
                    <div className="grid grid-cols-4 gap-4">
                      <div className="text-center">
                        <div className="text-2xl font-bold text-blue-600">
                          {stats.total_runs || 0}
                        </div>
                        <div className="text-sm text-muted-foreground">
                          {t('schedule.totalRuns')}
                        </div>
                      </div>
                      <div className="text-center">
                        <div className="text-2xl font-bold text-green-600">
                          {stats.successful_runs || 0}
                        </div>
                        <div className="text-sm text-muted-foreground">
                          {t('schedule.successfulRuns')}
                        </div>
                      </div>
                      <div className="text-center">
                        <div className="text-2xl font-bold text-red-600">
                          {stats.failed_runs || 0}
                        </div>
                        <div className="text-sm text-muted-foreground">
                          {t('schedule.failedRuns')}
                        </div>
                      </div>
                      <div className="text-center">
                        <Badge
                          variant={
                            stats.is_currently_running ? 'default' : 'secondary'
                          }
                        >
                          {stats.is_currently_running
                            ? t('schedule.running')
                            : t('schedule.idle')}
                        </Badge>
                        <div className="text-sm text-muted-foreground mt-2">
                          {t('schedule.currentStatus')}
                        </div>
                      </div>
                    </div>

                    {(stats as IScheduleStats).last_successful_run && (
                      <>
                        <Separator className="my-4" />
                        <div>
                          <span className="font-medium">
                            {t('schedule.lastSuccessfulRun')}:{' '}
                          </span>
                          <span className="text-muted-foreground">
                            {formatDateTime(
                              stats.last_successful_run.started_at,
                            )}
                          </span>
                        </div>
                      </>
                    )}
                  </>
                )}
              </CardContent>
            </Card>

            {/* Current Status Alert */}
            {stats.is_currently_running && (
              <div className="rounded-lg border border-blue-200 bg-blue-50 p-4">
                <div className="flex items-center">
                  <ClockCircleOutlined className="h-4 w-4 text-blue-600 mr-2" />
                  <div>
                    <div className="font-medium text-blue-900">
                      {t('schedule.currentlyRunning')}
                    </div>
                    <div className="text-sm text-blue-700">
                      {t('schedule.currentlyRunningDesc')}
                    </div>
                  </div>
                </div>
              </div>
            )}

            {/* Execution History */}
            <Card>
              <CardHeader>
                <CardTitle>{t('schedule.executionHistory')}</CardTitle>
              </CardHeader>
              <CardContent>
                {loadingHistory ? (
                  <div className="flex justify-center">
                    <Loader2 className="h-6 w-6 animate-spin" />
                  </div>
                ) : (
                  <div className="rounded-md border">
                    <Table>
                      <TableHeader>
                        <TableRow>
                          <TableHead>{t('schedule.startTime')}</TableHead>
                          <TableHead>{t('schedule.endTime')}</TableHead>
                          <TableHead>{t('schedule.duration')}</TableHead>
                          <TableHead>{t('schedule.status')}</TableHead>
                          <TableHead>{t('schedule.errorMessage')}</TableHead>
                        </TableRow>
                      </TableHeader>
                      <TableBody>
                        {history?.map((run: IScheduleRun) => (
                          <TableRow key={run.id}>
                            <TableCell>
                              {formatDateTime(run.started_at)}
                            </TableCell>
                            <TableCell>
                              {run.finished_at
                                ? formatDateTime(run.finished_at)
                                : t('schedule.running')}
                            </TableCell>
                            <TableCell>
                              {formatDuration(
                                calculateDuration(
                                  run.started_at,
                                  run.finished_at,
                                ),
                              )}
                            </TableCell>
                            <TableCell>{getStatusBadge(run)}</TableCell>
                            <TableCell>
                              {run.error_message ? (
                                <TooltipProvider>
                                  <Tooltip>
                                    <TooltipTrigger asChild>
                                      <span className="text-red-600 text-xs cursor-pointer">
                                        {run.error_message.slice(0, 30)}
                                        {run.error_message.length > 30
                                          ? '...'
                                          : ''}
                                      </span>
                                    </TooltipTrigger>
                                    <TooltipContent>
                                      <p>{run.error_message}</p>
                                    </TooltipContent>
                                  </Tooltip>
                                </TooltipProvider>
                              ) : (
                                '-'
                              )}
                            </TableCell>
                          </TableRow>
                        ))}
                      </TableBody>
                    </Table>
                  </div>
                )}
              </CardContent>
            </Card>
          </div>
        )}
      </SheetContent>
    </Sheet>
  );
}

interface ScheduleModalProps {
  visible: boolean;
  hideModal: () => void;
  canvasId: string;
  canvasTitle: string;
}

export function ScheduleModal({
  visible,
  hideModal,
  canvasId,
  canvasTitle,
}: ScheduleModalProps) {
  const { t } = useTranslate('flow');
  const [editingSchedule, setEditingSchedule] = useState<ISchedule | null>(
    null,
  );
  const [isFormVisible, setIsFormVisible] = useState(false);
  const [runDrawerVisible, setRunDrawerVisible] = useState(false);
  const [selectedSchedule, setSelectedSchedule] = useState<ISchedule | null>(
    null,
  );

  const { data: frequencyOptions, loading: loadingOptions } =
    useFetchFrequencyOptions();
  const {
    schedules,
    loading: loadingSchedules,
    refetch,
  } = useFetchSchedules(canvasId);
  const { toggleSchedule, loading: toggling } = useToggleSchedule();
  const { deleteSchedule, loading: deleting } = useDeleteSchedule();

  const handleCreateNew = useCallback(() => {
    setEditingSchedule(null);
    setIsFormVisible(true);
  }, []);

  const handleEdit = useCallback((schedule: ISchedule) => {
    setEditingSchedule(schedule);
    setIsFormVisible(true);
  }, []);

  const handleFormCancel = useCallback(() => {
    setIsFormVisible(false);
    setEditingSchedule(null);
  }, []);

  const handleFormSave = useCallback(() => {
    setIsFormVisible(false);
    setEditingSchedule(null);
    refetch();
  }, [refetch]);

  const handleToggle = useCallback(
    async (scheduleId: string) => {
      await toggleSchedule(scheduleId);
      refetch();
    },
    [toggleSchedule, refetch],
  );

  const handleDelete = useCallback(
    async (scheduleId: string) => {
      await deleteSchedule(scheduleId);
      refetch();
    },
    [deleteSchedule, refetch],
  );

  const handleViewRuns = useCallback((schedule: ISchedule) => {
    setSelectedSchedule(schedule);
    setRunDrawerVisible(true);
  }, []);

  const handleCloseRunDrawer = useCallback(() => {
    setRunDrawerVisible(false);
    setSelectedSchedule(null);
  }, []);

  // Show loading state
  if (loadingOptions) {
    return (
      <Dialog open={visible} onOpenChange={(open) => !open && hideModal()}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('schedule.title')}</DialogTitle>
          </DialogHeader>
          <div className="flex justify-center items-center h-32">
            <Loader2 className="h-8 w-8 animate-spin" />
          </div>
        </DialogContent>
      </Dialog>
    );
  }

  return (
    <>
      <Dialog open={visible} onOpenChange={(open) => !open && hideModal()}>
        <DialogContent className="max-w-6xl">
          <DialogHeader>
            <DialogTitle>{t('schedule.title')}</DialogTitle>
          </DialogHeader>

          <Card>
            <CardHeader>
              <div className="flex justify-between items-center">
                <CardTitle>
                  {t('schedule.for')} {canvasTitle}
                </CardTitle>
                <Button onClick={handleCreateNew} disabled={!frequencyOptions}>
                  {t('schedule.create')}
                </Button>
              </div>
            </CardHeader>
            <CardContent>
              {loadingSchedules ? (
                <div className="flex justify-center">
                  <Loader2 className="h-6 w-6 animate-spin" />
                </div>
              ) : (
                <div className="rounded-md border">
                  <Table>
                    <TableHeader>
                      <TableRow>
                        <TableHead>{t('schedule.name')}</TableHead>
                        <TableHead>{t('schedule.frequency')}</TableHead>
                        <TableHead>{t('schedule.status')}</TableHead>
                        <TableHead className="w-[120px]">
                          {t('common.action')}
                        </TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {schedules?.map((record: ISchedule) => (
                        <TableRow key={record.id}>
                          <TableCell>
                            <div>
                              <div className="font-medium">{record.name}</div>
                              {record.description && (
                                <div className="text-xs text-muted-foreground">
                                  {record.description}
                                </div>
                              )}
                            </div>
                          </TableCell>
                          <TableCell>
                            {(() => {
                              if (!frequencyOptions?.frequency_types) {
                                return record.frequency_type;
                              }

                              const option =
                                frequencyOptions.frequency_types.find(
                                  (t) => t.value === record.frequency_type,
                                );
                              let details =
                                option?.label || record.frequency_type;

                              if (
                                record.frequency_type === 'weekly' &&
                                record.days_of_week?.length &&
                                frequencyOptions?.days_of_week
                              ) {
                                const dayNames = record.days_of_week
                                  .map(
                                    (day) =>
                                      frequencyOptions.days_of_week.find(
                                        (d) => d.value === day,
                                      )?.label,
                                  )
                                  .filter(Boolean)
                                  .join(', ');
                                details += ` (${dayNames})`;
                              } else if (
                                record.frequency_type === 'monthly' &&
                                record.day_of_month
                              ) {
                                details += ` (${t('schedule.day')} ${record.day_of_month})`;
                              }

                              if (record.execute_time) {
                                details += ` ${t('schedule.at')} ${record.execute_time}`;
                              }

                              return details;
                            })()}
                          </TableCell>
                          <TableCell>
                            <div className="flex items-center space-x-2">
                              <Switch
                                checked={record.enabled}
                                onCheckedChange={() => handleToggle(record.id)}
                                disabled={toggling}
                              />
                              <Label className="text-sm">
                                {record.enabled
                                  ? t('schedule.enabled')
                                  : t('schedule.disabled')}
                              </Label>
                            </div>
                          </TableCell>
                          <TableCell>
                            <div className="flex items-center gap-1">
                              <TooltipProvider>
                                <Tooltip>
                                  <TooltipTrigger asChild>
                                    <Button
                                      variant="ghost"
                                      size="sm"
                                      onClick={() => handleViewRuns(record)}
                                    >
                                      <HistoryOutlined className="h-4 w-4" />
                                    </Button>
                                  </TooltipTrigger>
                                  <TooltipContent>
                                    <p>{t('schedule.viewRuns')}</p>
                                  </TooltipContent>
                                </Tooltip>
                              </TooltipProvider>

                              <TooltipProvider>
                                <Tooltip>
                                  <TooltipTrigger asChild>
                                    <Button
                                      variant="ghost"
                                      size="sm"
                                      onClick={() => handleEdit(record)}
                                    >
                                      <EditOutlined className="h-4 w-4" />
                                    </Button>
                                  </TooltipTrigger>
                                  <TooltipContent>
                                    <p>{t('common.edit')}</p>
                                  </TooltipContent>
                                </Tooltip>
                              </TooltipProvider>

                              <AlertDialog>
                                <TooltipProvider>
                                  <Tooltip>
                                    <TooltipTrigger asChild>
                                      <AlertDialogTrigger asChild>
                                        <Button
                                          variant="ghost"
                                          size="sm"
                                          disabled={deleting}
                                        >
                                          <DeleteOutlined className="h-4 w-4 text-red-500" />
                                        </Button>
                                      </AlertDialogTrigger>
                                    </TooltipTrigger>
                                    <TooltipContent>
                                      <p>{t('common.delete')}</p>
                                    </TooltipContent>
                                  </Tooltip>
                                </TooltipProvider>
                                <AlertDialogContent>
                                  <AlertDialogHeader>
                                    <AlertDialogTitle>
                                      {t('schedule.deleteConfirm')}
                                    </AlertDialogTitle>
                                    <AlertDialogDescription>
                                      This action cannot be undone. This will
                                      permanently delete the schedule.
                                    </AlertDialogDescription>
                                  </AlertDialogHeader>
                                  <AlertDialogFooter>
                                    <AlertDialogCancel>
                                      {t('common.no')}
                                    </AlertDialogCancel>
                                    <AlertDialogAction
                                      onClick={() => handleDelete(record.id)}
                                    >
                                      {t('common.yes')}
                                    </AlertDialogAction>
                                  </AlertDialogFooter>
                                </AlertDialogContent>
                              </AlertDialog>
                            </div>
                          </TableCell>
                        </TableRow>
                      ))}
                    </TableBody>
                  </Table>
                </div>
              )}
            </CardContent>
          </Card>
        </DialogContent>
      </Dialog>

      <ScheduleFormModal
        visible={isFormVisible}
        onCancel={handleFormCancel}
        onSave={handleFormSave}
        editingSchedule={editingSchedule}
        canvasId={canvasId}
        loading={loadingSchedules}
      />

      <ScheduleRunDrawer
        visible={runDrawerVisible}
        onClose={handleCloseRunDrawer}
        schedule={selectedSchedule}
      />
    </>
  );
}

export function useScheduleModal() {
  const [visible, setVisible] = useState(false);

  const showModal = useCallback(() => setVisible(true), []);
  const hideModal = useCallback(() => setVisible(false), []);

  return { visible, showModal, hideModal };
}