/* eslint-disable react-hooks/exhaustive-deps */

'use client';

import {
  Children,
  createContext,
  HTMLAttributes,
  isValidElement,
  ReactElement,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
} from 'react';

import { cn } from '@/lib/utils';

// Types
type StepperOrientation = 'horizontal' | 'vertical';
type StepState = 'active' | 'completed' | 'inactive' | 'loading';
type StepIndicators = {
  active?: React.ReactNode;
  completed?: React.ReactNode;
  inactive?: React.ReactNode;
  loading?: React.ReactNode;
};

interface StepperContextValue {
  activeStep: number;
  setActiveStep: (step: number) => void;
  stepsCount: number;
  orientation: StepperOrientation;
  registerTrigger: (node: HTMLButtonElement | null) => void;
  triggerNodes: HTMLButtonElement[];
  focusNext: (currentIdx: number) => void;
  focusPrev: (currentIdx: number) => void;
  focusFirst: () => void;
  focusLast: () => void;
  indicators: StepIndicators;
}

interface StepItemContextValue {
  step: number;
  state: StepState;
  isDisabled: boolean;
  isLoading: boolean;
}

const StepperContext = createContext<StepperContextValue | undefined>(
  undefined,
);
const StepItemContext = createContext<StepItemContextValue | undefined>(
  undefined,
);

function useStepper() {
  const ctx = useContext(StepperContext);
  if (!ctx) throw new Error('useStepper must be used within a Stepper');
  return ctx;
}

function useStepItem() {
  const ctx = useContext(StepItemContext);
  if (!ctx) throw new Error('useStepItem must be used within a StepperItem');
  return ctx;
}

interface StepperProps extends HTMLAttributes<HTMLDivElement> {
  defaultValue?: number;
  value?: number;
  onValueChange?: (value: number) => void;
  orientation?: StepperOrientation;
  indicators?: StepIndicators;
}

function Stepper({
  defaultValue = 1,
  value,
  onValueChange,
  orientation = 'horizontal',
  className,
  children,
  indicators = {},
  ...props
}: StepperProps) {
  const [activeStep, setActiveStep] = useState(defaultValue);
  const [triggerNodes, setTriggerNodes] = useState<HTMLButtonElement[]>([]);

  // Register/unregister triggers
  const registerTrigger = useCallback((node: HTMLButtonElement | null) => {
    setTriggerNodes((prev) => {
      if (node && !prev.includes(node)) {
        return [...prev, node];
      } else if (!node && prev.includes(node!)) {
        return prev.filter((n) => n !== node);
      } else {
        return prev;
      }
    });
  }, []);

  const handleSetActiveStep = useCallback(
    (step: number) => {
      if (value === undefined) {
        setActiveStep(step);
      }
      onValueChange?.(step);
    },
    [value, onValueChange],
  );

  const currentStep = value ?? activeStep;

  // Keyboard navigation logic
  const focusTrigger = (idx: number) => {
    if (triggerNodes[idx]) triggerNodes[idx].focus();
  };
  const focusNext = (currentIdx: number) =>
    focusTrigger((currentIdx + 1) % triggerNodes.length);
  const focusPrev = (currentIdx: number) =>
    focusTrigger((currentIdx - 1 + triggerNodes.length) % triggerNodes.length);
  const focusFirst = () => focusTrigger(0);
  const focusLast = () => focusTrigger(triggerNodes.length - 1);

  // Context value
  const contextValue = useMemo<StepperContextValue>(
    () => ({
      activeStep: currentStep,
      setActiveStep: handleSetActiveStep,
      stepsCount: Children.toArray(children).filter(
        (child): child is ReactElement =>
          isValidElement(child) &&
          (child.type as { displayName?: string }).displayName ===
            'StepperItem',
      ).length,
      orientation,
      registerTrigger,
      focusNext,
      focusPrev,
      focusFirst,
      focusLast,
      triggerNodes,
      indicators,
    }),
    [
      currentStep,
      handleSetActiveStep,
      children,
      orientation,
      registerTrigger,
      triggerNodes,
    ],
  );

  return (
    <StepperContext.Provider value={contextValue}>
      <div
        role="tablist"
        aria-orientation={orientation}
        data-slot="stepper"
        className={cn('w-full', className)}
        data-orientation={orientation}
        {...props}
      >
        {children}
      </div>
    </StepperContext.Provider>
  );
}

interface StepperItemProps extends React.HTMLAttributes<HTMLDivElement> {
  step: number;
  completed?: boolean;
  disabled?: boolean;
  loading?: boolean;
}

function StepperItem({
  step,
  completed = false,
  disabled = false,
  loading = false,
  className,
  children,
  ...props
}: StepperItemProps) {
  const { activeStep } = useStepper();

  const state: StepState =
    completed || step < activeStep
      ? 'completed'
      : activeStep === step
        ? 'active'
        : 'inactive';

  const isLoading = loading && step === activeStep;

  return (
    <StepItemContext.Provider
      value={{ step, state, isDisabled: disabled, isLoading }}
    >
      <div
        data-slot="stepper-item"
        className={cn(
          'group/step flex items-center justify-center not-last:flex-1 group-data-[orientation=horizontal]/stepper-nav:flex-row group-data-[orientation=vertical]/stepper-nav:flex-col',
          className,
        )}
        data-state={state}
        {...(isLoading ? { 'data-loading': true } : {})}
        {...props}
      >
        {children}
      </div>
    </StepItemContext.Provider>
  );
}

interface StepperTriggerProps extends React.ButtonHTMLAttributes<HTMLButtonElement> {
  asChild?: boolean;
}

function StepperTrigger({
  asChild = false,
  className,
  children,
  tabIndex,
  ...props
}: StepperTriggerProps) {
  const { state, isLoading } = useStepItem();
  const stepperCtx = useStepper();
  const {
    setActiveStep,
    activeStep,
    registerTrigger,
    triggerNodes,
    focusNext,
    focusPrev,
    focusFirst,
    focusLast,
  } = stepperCtx;
  const { step, isDisabled } = useStepItem();
  const isSelected = activeStep === step;
  const id = `stepper-tab-${step}`;
  const panelId = `stepper-panel-${step}`;

  // Register this trigger for keyboard navigation
  const btnRef = useRef<HTMLButtonElement>(null);
  useEffect(() => {
    if (btnRef.current) {
      registerTrigger(btnRef.current);
    }
  }, [btnRef.current]);

  // Find our index among triggers for navigation
  const myIdx = useMemo(
    () =>
      triggerNodes.findIndex((n: HTMLButtonElement) => n === btnRef.current),
    [triggerNodes, btnRef.current],
  );

  const handleKeyDown = (e: React.KeyboardEvent<HTMLButtonElement>) => {
    switch (e.key) {
      case 'ArrowRight':
      case 'ArrowDown':
        e.preventDefault();
        if (myIdx !== -1 && focusNext) focusNext(myIdx);
        break;
      case 'ArrowLeft':
      case 'ArrowUp':
        e.preventDefault();
        if (myIdx !== -1 && focusPrev) focusPrev(myIdx);
        break;
      case 'Home':
        e.preventDefault();
        if (focusFirst) focusFirst();
        break;
      case 'End':
        e.preventDefault();
        if (focusLast) focusLast();
        break;
      case 'Enter':
      case ' ':
        e.preventDefault();
        setActiveStep(step);
        break;
    }
  };

  if (asChild) {
    return (
      <span
        data-slot="stepper-trigger"
        data-state={state}
        className={className}
      >
        {children}
      </span>
    );
  }

  return (
    <button
      ref={btnRef}
      role="tab"
      id={id}
      aria-selected={isSelected}
      aria-controls={panelId}
      tabIndex={typeof tabIndex === 'number' ? tabIndex : isSelected ? 0 : -1}
      data-slot="stepper-trigger"
      data-state={state}
      data-loading={isLoading}
      className={cn(
        'focus-visible:border-ring focus-visible:ring-ring/50 inline-flex cursor-pointer items-center outline-none focus-visible:z-10 focus-visible:ring-3 disabled:pointer-events-none disabled:opacity-60',
        'gap-2.5 rounded-full',
        className,
      )}
      onClick={() => setActiveStep(step)}
      onKeyDown={handleKeyDown}
      disabled={isDisabled}
      {...props}
    >
      {children}
    </button>
  );
}

function StepperIndicator({
  children,
  className,
}: React.ComponentProps<'div'>) {
  const { state, isLoading } = useStepItem();
  const { indicators } = useStepper();

  return (
    <div
      data-slot="stepper-indicator"
      data-state={state}
      className={cn(
        'border-background bg-accent text-accent-foreground data-[state=completed]:bg-primary data-[state=completed]:text-primary-foreground data-[state=active]:bg-primary data-[state=active]:text-primary-foreground relative flex size-6 shrink-0 items-center justify-center overflow-hidden',
        'rounded-full text-xs',
        className,
      )}
    >
      <div className="absolute">
        {indicators &&
        ((isLoading && indicators.loading) ||
          (state === 'completed' && indicators.completed) ||
          (state === 'active' && indicators.active) ||
          (state === 'inactive' && indicators.inactive))
          ? (isLoading && indicators.loading) ||
            (state === 'completed' && indicators.completed) ||
            (state === 'active' && indicators.active) ||
            (state === 'inactive' && indicators.inactive)
          : children}
      </div>
    </div>
  );
}

function StepperSeparator({ className }: React.ComponentProps<'div'>) {
  const { state } = useStepItem();

  return (
    <div
      data-slot="stepper-separator"
      data-state={state}
      className={cn(
        'bg-muted rounded-sm group-data-[orientation=horizontal]/stepper-nav:h-0.5 group-data-[orientation=vertical]/stepper-nav:h-12 group-data-[orientation=vertical]/stepper-nav:w-0.5 m-0.5 group-data-[orientation=horizontal]/stepper-nav:flex-1',
        className,
      )}
    />
  );
}

function StepperTitle({ children, className }: React.ComponentProps<'h3'>) {
  const { state } = useStepItem();

  return (
    <h3
      data-slot="stepper-title"
      data-state={state}
      className={cn('text-sm leading-none font-medium', className)}
    >
      {children}
    </h3>
  );
}

function StepperDescription({
  children,
  className,
}: React.ComponentProps<'div'>) {
  const { state } = useStepItem();

  return (
    <div
      data-slot="stepper-description"
      data-state={state}
      className={cn('text-muted-foreground text-sm', className)}
    >
      {children}
    </div>
  );
}

function StepperNav({ children, className }: React.ComponentProps<'nav'>) {
  const { activeStep, orientation } = useStepper();

  return (
    <nav
      data-slot="stepper-nav"
      data-state={activeStep}
      data-orientation={orientation}
      className={cn(
        'group/stepper-nav inline-flex data-[orientation=horizontal]:w-full data-[orientation=horizontal]:flex-row data-[orientation=vertical]:flex-col',
        className,
      )}
    >
      {children}
    </nav>
  );
}

function StepperPanel({ children, className }: React.ComponentProps<'div'>) {
  const { activeStep } = useStepper();

  return (
    <div
      data-slot="stepper-panel"
      data-state={activeStep}
      className={cn('w-full', className)}
    >
      {children}
    </div>
  );
}

interface StepperContentProps extends React.ComponentProps<'div'> {
  value: number;
  forceMount?: boolean;
}

function StepperContent({
  value,
  forceMount,
  children,
  className,
}: StepperContentProps) {
  const { activeStep } = useStepper();
  const isActive = value === activeStep;

  if (!forceMount && !isActive) {
    return null;
  }

  return (
    <div
      data-slot="stepper-content"
      data-state={activeStep}
      className={cn('w-full', className, !isActive && forceMount && 'hidden')}
      hidden={!isActive && forceMount}
    >
      {children}
    </div>
  );
}

export {
  Stepper,
  StepperContent,
  StepperDescription,
  StepperIndicator,
  StepperItem,
  StepperNav,
  StepperPanel,
  StepperSeparator,
  StepperTitle,
  StepperTrigger,
  useStepItem,
  useStepper,
  type StepperContentProps,
  type StepperItemProps,
  type StepperProps,
  type StepperTriggerProps,
};
