import { Input } from '@/components/originui/input';
import { useTranslate } from '@/hooks/common-hooks';
import { EyeIcon, EyeOffIcon } from 'lucide-react';
import { ChangeEvent, LegacyRef, forwardRef, useId, useState } from 'react';

type PropType = {
  name: string;
  value: string;
  onBlur: () => void;
  onChange: (event: ChangeEvent<HTMLInputElement>) => void;
};

function PasswordInput(
  props: PropType,
  ref: LegacyRef<HTMLInputElement> | undefined,
) {
  const id = useId();
  const [isVisible, setIsVisible] = useState<boolean>(false);

  const toggleVisibility = () => setIsVisible((prevState) => !prevState);

  const { t } = useTranslate('setting');

  return (
    <div className="*:not-first:mt-2 w-full">
      {/* <Label htmlFor={id}>Show/hide password input</Label> */}
      <div className="relative">
        <Input
          autoComplete="off"
          inputMode="numeric"
          id={id}
          className="pe-9"
          placeholder=""
          type={isVisible ? 'text' : 'password'}
          value={props.value}
          onBlur={props.onBlur}
          onChange={(ev) => props.onChange(ev)}
        />
        <button
          className="text-muted-foreground/80 hover:text-foreground focus-visible:border-ring focus-visible:ring-ring/50 absolute inset-y-0 end-0 flex h-full w-9 items-center justify-center rounded-e-md transition-[color,box-shadow] outline-none focus:z-10 focus-visible:ring-[3px] disabled:pointer-events-none disabled:cursor-not-allowed disabled:opacity-50"
          type="button"
          onClick={toggleVisibility}
          aria-label={isVisible ? 'Hide password' : 'Show password'}
          aria-pressed={isVisible}
          aria-controls="password"
        >
          {isVisible ? (
            <EyeOffIcon size={16} aria-hidden="true" />
          ) : (
            <EyeIcon size={16} aria-hidden="true" />
          )}
        </button>
      </div>
    </div>
  );
}

export default forwardRef(PasswordInput);
